package servicehealth

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/servicehealth/health"
	"github.com/aiops/aiops-platform/internal/servicehealth/signals"
)

// HealthResult 是 Service Health API 的组合返回结果。
// 包含 Service 基本信息、Health Evaluation、Signal Trust 分布和 Source Errors。
type HealthResult struct {
	Service      ServiceInfo       `json:"service"`
	Health       health.HealthEvaluation `json:"health"`
	Signals      SignalCounts      `json:"signals"`
	SourceErrors map[string]string `json:"source_errors,omitempty"`
}

// ServiceInfo 是 HealthResult 中的 Service 基本信息（不暴露完整 Service Model）。
type ServiceInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`
}

// SignalCounts 是 Signal Trust 分布统计（observability metadata，不是 Health Score）。
type SignalCounts struct {
	Total int `json:"total"`
	Fresh int `json:"fresh"`
	Stale int `json:"stale"`
	Error int `json:"error"`
	Empty int `json:"empty"`
}

// countSignals 统计 Evidence 的 TrustStatus 分布。
func countSignals(evidences []rca.Evidence) SignalCounts {
	counts := SignalCounts{Total: len(evidences)}
	for _, e := range evidences {
		switch e.TrustStatus {
		case "fresh":
			counts.Fresh++
		case "stale":
			counts.Stale++
		case "error":
			counts.Error++
		case "empty":
			counts.Empty++
		}
	}
	return counts
}

// Manager 是 Service Health 的业务服务层，
// 协调 Repository（持久化）、DiscoveryService（K8s 发现）、SignalCollectorManager（信号采集）
// 和 HealthEvaluator（健康评估）。
type Manager struct {
	repo           *Repository
	discovery      *DiscoveryService
	signalsManager *signals.SignalCollectorManager
	evaluator      health.HealthEvaluator
}

// NewManager 创建 Service Health 业务层。
// signalsManager 可为 nil（此时 CollectSignals 返回错误）。
// evaluator 可为 nil（此时 EvaluateHealth 使用默认 DefaultEvaluator）。
func NewManager(repo *Repository, discovery *DiscoveryService, signalsManager *signals.SignalCollectorManager, evaluator health.HealthEvaluator) *Manager {
	if evaluator == nil {
		evaluator = health.NewDefaultEvaluator()
	}
	return &Manager{repo: repo, discovery: discovery, signalsManager: signalsManager, evaluator: evaluator}
}

// List 执行 Sync（Discovery + Upsert）后查询 Service 列表。
// 每次调用确保数据来自最新 K8s 状态，同时持久化到 DB。
//
// Error != Empty:
//   - K8s API 失败 → 返回 error（不返回空列表）
//   - DB 失败 → 返回 error
//   - K8s 正常但无 Service → 返回空列表（HTTP 200 + []）
func (s *Manager) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Service, int64, error) {
	// 先执行 Sync，确保 DB 中有最新 K8s 数据
	if filter.Cluster != "" {
		if _, err := s.Sync(ctx, filter.Cluster, filter.Namespace); err != nil {
			return nil, 0, err
		}
	}
	return s.repo.List(ctx, filter, page, pageSize)
}

// GetByID 按主键查询 Service。
// 不触发 Discovery，直接从 DB 查询。
func (s *Manager) GetByID(ctx context.Context, id int64) (*Service, error) {
	return s.repo.FindByID(ctx, id)
}

// GetByIDAndCluster 按主键 + cluster 查询 Service。
// P2-01 Phase 3 G5: Service ID 不能单独决定访问权限，必须校验 cluster。
func (s *Manager) GetByIDAndCluster(ctx context.Context, id int64, cluster string) (*Service, error) {
	return s.repo.FindByIDAndCluster(ctx, id, cluster)
}

// CollectSignals 采集指定 Service 的 Health Signals。
//
// 流程：
//  1. 按 id + cluster 查询 Service（G5 cluster 隔离）
//  2. 从 K8s 获取该 Service 所在 namespace 的 Pods
//  3. 通过 selector 过滤出属于该 Service 的 Pods
//  4. 调用 SignalCollectorManager 并行采集所有数据源
//
// Error != Empty:
//   - Service 不存在 → 返回 nil, nil（调用方返回 404）
//   - cluster 不匹配 → 返回 nil, nil（调用方返回 404，防止越权）
//   - signalsManager 未配置 → 返回 error
//   - 所有 Collector 都失败 → 返回 result + error（调用方返回 503）
//   - 部分 Collector 失败 → 返回 result（包含 signals + source_errors），error=nil
//   - 所有 Collector 成功但无数据 → 返回 result（空 signals），error=nil
func (s *Manager) CollectSignals(ctx context.Context, id int64, cluster string) (*signals.SignalCollectionResult, error) {
	if s.signalsManager == nil {
		return nil, errors.New("signal collector manager not configured")
	}

	// 1. 按 id + cluster 查询 Service（G5）
	svc, err := s.repo.FindByIDAndCluster(ctx, id, cluster)
	if err != nil {
		return nil, err
	}
	if svc == nil {
		return nil, nil // not found or cluster mismatch
	}

	// 2. 从 K8s 获取该 Service 所在 namespace 的 Pods
	allPods, err := s.discovery.ListPods(ctx, cluster, svc.Namespace)
	if err != nil {
		// K8s API 失败 → 不产生 fake pods，但仍然可以让其他 Collector 尝试
		// 这里返回 error，因为 K8s 是主要数据源
		return nil, err
	}

	// 3. 通过 selector 过滤出属于该 Service 的 Pods
	selector := jsonToSelector(svc.WorkloadSelector)
	var servicePods []corev1.Pod
	if len(selector) > 0 {
		for i := range allPods {
			if labelsMatch(selector, allPods[i].Labels) {
				servicePods = append(servicePods, allPods[i])
			}
		}
	} else {
		// 无 selector 的 Service（如 ExternalName），不关联 Pods
		servicePods = nil
	}

	// 4. 调用 SignalCollectorManager 并行采集
	return s.signalsManager.CollectForService(
		ctx,
		svc.ID,
		svc.Name,
		svc.Namespace,
		svc.Cluster,
		string(svc.WorkloadType),
		svc.WorkloadName,
		selector,
		servicePods,
	)
}

// Sync 执行 Kubernetes Discovery 并将结果 Upsert 到数据库。
//
// 策略：
//   - Discovery 与 Persistence 解耦
//   - Upsert 使用 cluster+namespace+name 作为业务 identity
//   - K8s 中已删除的 Service：本阶段不物理删除，保留在 DB
//   - 返回 discovered 的 Service 数量
func (s *Manager) Sync(ctx context.Context, cluster, namespace string) (int, error) {
	discovered, err := s.discovery.Discover(ctx, cluster, namespace)
	if err != nil {
		return 0, err
	}

	for i := range discovered {
		d := &discovered[i]
		svc := &Service{
			Name:             d.Name,
			Namespace:        d.Namespace,
			Cluster:          d.Cluster,
			WorkloadType:     d.WorkloadType,
			WorkloadName:     d.WorkloadName,
			WorkloadSelector: selectorToJSON(d.WorkloadSelector),
			ServiceType:      d.ServiceType,
		}
		if _, err := s.repo.Upsert(ctx, svc); err != nil {
			return i, err
		}
	}
	return len(discovered), nil
}

// SyncAndList 执行 Sync 后返回指定 cluster/namespace 的所有 Service（不分页）。
// 用于需要完整列表的场景。
func (s *Manager) SyncAndList(ctx context.Context, cluster, namespace string) ([]Service, error) {
	if _, err := s.Sync(ctx, cluster, namespace); err != nil {
		return nil, err
	}
	if namespace != "" {
		return s.repo.ListByNamespace(ctx, cluster, namespace)
	}
	return s.repo.ListByCluster(ctx, cluster)
}

// EvaluateHealth 采集 Signals 并执行 Health Evaluation，返回组合结果。
//
// 流程：
//  1. 调用 CollectSignals 采集所有数据源的 Evidence
//  2. 调用 HealthEvaluator 评估 Health State（复用 Phase 4，不在 Handler 中重新判断）
//  3. 统计 Signal Trust 分布（fresh/stale/error/empty）
//  4. 保留 source_errors
//
// 错误处理：
//   - Service 不存在或 cluster 不匹配 → 返回 nil, nil（调用方返回 404）
//   - signalsManager 未配置 → 返回 error
//   - 所有 Collector 都失败 → 仍返回结果（health=unknown, source_errors 完整），不返回 503
//     因为 unknown 是一种有效的 Health State
//   - K8s API 失败 → 返回 error（主要数据源不可用）
//
// 核心语义：
//   - Health State 完全来自 Phase 4 HealthEvaluator，不在此方法中重新判断
//   - no data != healthy, stale != healthy, error != healthy
//   - source_errors 存在不强制改变 Health State
func (s *Manager) EvaluateHealth(ctx context.Context, id int64, cluster string) (*HealthResult, error) {
	// 1. 采集 Signals
	signalResult, err := s.CollectSignals(ctx, id, cluster)
	if err != nil {
		// K8s API 失败或 signalsManager 未配置 → 返回 error
		// 但 ErrAllCollectorsFailed 时 signalResult 可能非 nil，继续评估
		if !errors.Is(err, signals.ErrAllCollectorsFailed) {
			return nil, err
		}
		// ErrAllCollectorsFailed: signalResult 中有 source_errors，继续评估（signals 为空 → unknown）
	}
	if signalResult == nil {
		// Service 不存在或 cluster 不匹配
		return nil, nil
	}

	// 2. 执行 Health Evaluation（复用 Phase 4 Evaluator，不在此重新判断）
	evaluation := s.evaluator.Evaluate(signalResult.Signals)

	// 3. 获取 Service 基本信息（从 CollectSignals 内部已查询，但未返回，这里重新查询）
	svc, err := s.repo.FindByIDAndCluster(ctx, id, cluster)
	if err != nil {
		return nil, err
	}
	if svc == nil {
		return nil, nil
	}

	// 4. 组装结果
	return &HealthResult{
		Service: ServiceInfo{
			ID:        svc.ID,
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Cluster:   svc.Cluster,
		},
		Health:       evaluation,
		Signals:      countSignals(signalResult.Signals),
		SourceErrors: signalResult.SourceErrors,
	}, nil
}
