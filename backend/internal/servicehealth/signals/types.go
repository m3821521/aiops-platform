// Package signals 实现 Service Health Signal Collector。
//
// 核心原则：
//   - Signal ≠ Health State。Collector 只负责采集原始观测并转换为 rca.Evidence。
//   - 所有 Signal 统一使用 rca.Evidence，不创建第二套 Evidence 模型。
//   - 每条 Evidence 必须具备完整 provenance（source/sourceType/fetchedAt/dataTimestamp/trustStatus/causalRelevance）。
//   - Error != Empty。API 失败不能产生 value=0 的 fake signal。
//   - 支持多数据源 partial failure。
package signals

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/rca"
)

// ErrAllCollectorsFailed 表示所有 SignalCollector 都失败了。
// 此时 SignalCollectionResult 中只有 source_errors，没有 signals。
var ErrAllCollectorsFailed = errors.New("all signal collectors failed")

// ServiceContext 是传递给 SignalCollector 的 Service 信息。
// 使用独立结构体而非 *servicehealth.Service，避免 signals → servicehealth 循环依赖。
// servicehealth.Manager 在调用 Collector 时将 *Service 转换为此结构。
type ServiceContext struct {
	ID               int64
	Name             string
	Namespace        string
	Cluster          string
	WorkloadType     string
	WorkloadName     string
	WorkloadSelector map[string]string // 已反序列化的 selector
}

// SignalCollector 是单个数据源的 Signal 采集器接口。
// 每个实现负责从一个数据源（K8s/Prometheus/Alert/ES/Topology）采集原始观测，
// 转换为 []rca.Evidence，并附带完整 provenance。
//
// Collect 返回：
//   - evidences: 成功采集到的 Evidence 列表（可能为空，表示该数据源无匹配数据）
//   - err: 数据源完全失败时返回非 nil error（将进入 source_errors）
//
// 部分成功（如部分 Pod 有数据、部分没有）应返回 evidences + nil error。
// 完全无数据（API 成功但无匹配）应返回 nil + nil（empty，不是 error）。
type SignalCollector interface {
	Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error)
	// Source 返回数据源标识，用于 provenance.source 和 source_errors key。
	Source() string
}

// SignalCollectionResult 是 SignalCollectorManager 的聚合结果。
// 支持 partial failure：部分数据源成功、部分失败时，HTTP 仍返回 200，
// 通过 source_errors 标注失败的数据源。
type SignalCollectionResult struct {
	Signals      []rca.Evidence    `json:"signals"`
	SourceErrors map[string]string `json:"source_errors,omitempty"`
	FetchedAt    time.Time         `json:"fetched_at"`
}

// HasSignals 判断是否有成功采集到的 Signal。
func (r *SignalCollectionResult) HasSignals() bool {
	return len(r.Signals) > 0
}

// AllFailed 判断是否所有数据源都失败（无 signal 且有 source_errors）。
func (r *SignalCollectionResult) AllFailed() bool {
	return len(r.Signals) == 0 && len(r.SourceErrors) > 0
}
