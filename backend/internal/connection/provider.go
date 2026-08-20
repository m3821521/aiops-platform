package connection

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ConnectionProvider 是所有外部系统连接 Provider 的基础接口。
//
// 设计原则：
//   - 业务 Service 依赖 Provider 接口，不依赖具体 Client
//   - 每个 Connection 类型对应一个 Provider 实现
//   - Provider 负责连接测试和客户端创建
//   - 具体的业务操作由更具体的 Provider 接口定义（如 KubernetesProvider、MetricsProvider）
//
// Phase A 只定义接口和 Registry，不实现具体 Adapter。
// 具体 Provider 实现在 Phase B-F 中逐步迁移。
type ConnectionProvider interface {
	// Type 返回 Provider 对应的 Connection 类型。
	Type() ConnectionType

	// Test 测试连接是否可用。
	// 返回 TestResult，包含状态、延迟、错误信息。
	Test(ctx context.Context, conn *Connection) (*TestConnectionResult, error)

	// Connect 创建并返回连接客户端。
	// 返回的客户端类型由具体 Provider 定义，业务层应通过类型断言获取。
	// 注意：业务层应优先使用具体 Provider 接口（如 KubernetesProvider），而不是直接调用 Connect。
	Connect(ctx context.Context, conn *Connection) (interface{}, error)
}

// ProviderRegistry 是 ConnectionProvider 的注册中心。
//
// 功能：
//   - 注册和获取 Provider
//   - 按 Connection 类型查找 Provider
//   - 执行连接测试
//
// 线程安全：支持并发注册和查询。
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[ConnectionType]ConnectionProvider
}

// NewProviderRegistry 创建 Provider Registry。
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[ConnectionType]ConnectionProvider),
	}
}

// Register 注册一个 ConnectionProvider。
// 如果同类型的 Provider 已存在，则覆盖。
func (r *ProviderRegistry) Register(provider ConnectionProvider) error {
	if provider == nil {
		return errors.New("provider 不能为空")
	}
	if provider.Type() == "" {
		return errors.New("provider type 不能为空")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Type()] = provider
	return nil
}

// Get 根据 Connection 类型获取 Provider。
func (r *ProviderRegistry) Get(connType ConnectionType) (ConnectionProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[connType]
	if !ok {
		return nil, errors.New("未注册的 connection type: " + string(connType))
	}
	return provider, nil
}

// Has 检查是否注册了指定类型的 Provider。
func (r *ProviderRegistry) Has(connType ConnectionType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[connType]
	return ok
}

// List 返回所有已注册的 Provider 类型。
func (r *ProviderRegistry) List() []ConnectionType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]ConnectionType, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}

// TestConnection 测试指定 Connection 的连通性。
// 如果对应类型的 Provider 未注册，则返回错误。
func (r *ProviderRegistry) TestConnection(ctx context.Context, conn *Connection) (*TestConnectionResult, error) {
	if conn == nil {
		return nil, errors.New("connection 不能为空")
	}

	provider, err := r.Get(conn.Type)
	if err != nil {
		// Provider 未注册，返回 unknown 状态
		return &TestConnectionResult{
			Status:       StatusUnknown,
			LatencyMs:    0,
			ErrorCode:    "PROVIDER_NOT_REGISTERED",
			ErrorMessage: "该连接类型的 Provider 尚未实现: " + string(conn.Type),
			CheckedAt:    time.Now(),
		}, nil
	}

	start := time.Now()
	result, err := provider.Test(ctx, conn)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &TestConnectionResult{
			Status:       StatusUnavailable,
			LatencyMs:    latency,
			ErrorCode:    "TEST_FAILED",
			ErrorMessage: err.Error(),
			CheckedAt:    time.Now(),
		}, nil
	}

	if result == nil {
		return &TestConnectionResult{
			Status:       StatusUnknown,
			LatencyMs:    latency,
			ErrorCode:    "EMPTY_RESULT",
			ErrorMessage: "Provider 返回空结果",
			CheckedAt:    time.Now(),
		}, nil
	}

	result.LatencyMs = latency
	result.CheckedAt = time.Now()
	return result, nil
}

// ============================================================================
// 具体 Provider 接口定义
// ============================================================================
// 以下接口定义各类型 Connection 的业务操作。
// Phase A 只定义接口，不实现具体 Adapter。
// 具体实现在 Phase B-F 中逐步迁移。
// ============================================================================

// KubernetesProvider 是 Kubernetes 连接的 Provider 接口。
//
// 业务 Service 应依赖此接口，而不是直接使用 client-go。
type KubernetesProvider interface {
	ConnectionProvider

	// ListNodes 获取节点列表。
	ListNodes(ctx context.Context, conn *Connection) ([]interface{}, error)

	// ListPods 获取 Pod 列表。
	ListPods(ctx context.Context, conn *Connection, namespace string) ([]interface{}, error)

	// GetPod 获取单个 Pod 详情。
	GetPod(ctx context.Context, conn *Connection, namespace, name string) (interface{}, error)

	// RestartPod 重启 Pod（高风险操作，需要审批）。
	RestartPod(ctx context.Context, conn *Connection, namespace, name string) error
}

// MetricsProvider 是监控指标连接的 Provider 接口。
//
// 当前实现：PrometheusProvider
// 未来扩展：VictoriaMetricsProvider、ThanosProvider 等
type MetricsProvider interface {
	ConnectionProvider

	// Query 执行即时查询。
	Query(ctx context.Context, conn *Connection, query string, timestamp time.Time) (interface{}, error)

	// QueryRange 执行范围查询。
	QueryRange(ctx context.Context, conn *Connection, query string, start, end time.Time, step time.Duration) (interface{}, error)
}

// LogProvider 是日志连接的 Provider 接口。
//
// 当前实现：ElasticsearchProvider
// 未来扩展：LokiProvider、OpenSearchProvider 等
type LogProvider interface {
	ConnectionProvider

	// Search 搜索日志。
	Search(ctx context.Context, conn *Connection, query string, startTime, endTime time.Time, limit int) (interface{}, error)
}

// CIProvider 是 CI 系统连接的 Provider 接口。
//
// 当前实现：JenkinsProvider
// 未来扩展：GitLabCIProvider、GitHubActionsProvider 等
type CIProvider interface {
	ConnectionProvider

	// TriggerBuild 触发构建。
	TriggerBuild(ctx context.Context, conn *Connection, job string, parameters map[string]string) (interface{}, error)

	// GetBuild 获取构建状态。
	GetBuild(ctx context.Context, conn *Connection, job string, buildNumber int) (interface{}, error)

	// GetBuildLog 获取构建日志。
	GetBuildLog(ctx context.Context, conn *Connection, job string, buildNumber int) (string, error)
}

// CDProvider 是 CD 系统连接的 Provider 接口。
//
// 当前实现：ArgoCDProvider
// 未来扩展：FluxProvider、SpinnakerProvider 等
type CDProvider interface {
	ConnectionProvider

	// GetApplication 获取应用详情。
	GetApplication(ctx context.Context, conn *Connection, name string) (interface{}, error)

	// SyncApplication 同步应用。
	SyncApplication(ctx context.Context, conn *Connection, name string) (interface{}, error)

	// GetSyncStatus 获取同步状态。
	GetSyncStatus(ctx context.Context, conn *Connection, name string) (interface{}, error)
}
