package topology

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// StatusProvider 是状态计算的数据源接口。
// 从 Incident/Alert/Anomaly 获取资源状态。
type StatusProvider interface {
	// ResourceStatus 返回指定资源的状态和关联的 Incident ID。
	ResourceStatus(ctx context.Context, cluster, resourceType, namespace, name string) (NodeStatus, []int64, int, int)
}

// Service 是拓扑业务逻辑层。
type Service struct {
	builder        *Builder
	rdb            *redis.Client
	statusProvider StatusProvider
	cacheTTL       time.Duration
}

// NewService 创建拓扑 Service。
func NewService(builder *Builder, rdb *redis.Client, statusProvider StatusProvider) *Service {
	return &Service{
		builder:        builder,
		rdb:            rdb,
		statusProvider: statusProvider,
		cacheTTL:       60 * time.Second,
	}
}

// SetCacheTTL 设置缓存 TTL。
func (s *Service) SetCacheTTL(ttl time.Duration) {
	s.cacheTTL = ttl
}

// cacheKey 生成缓存 key。
func (s *Service) cacheKey(cluster, namespace string) string {
	if namespace == "" {
		return "topology:" + cluster
	}
	return "topology:" + cluster + ":" + namespace
}

// cacheEnvelope 是 Redis 缓存的内部包装结构，包含 Graph 和缓存创建时间。
// 用于 provenance：cacheCreatedAt 真实来自缓存写入时间，禁止伪造。
type cacheEnvelope struct {
	Graph     Graph     `json:"graph"`
	CreatedAt time.Time `json:"createdAt"`
}

// CacheProvenance 描述拓扑图的缓存来源信息。
type CacheProvenance struct {
	// Hit 是否命中 Redis cache
	Hit bool
	// CreatedAt 缓存创建时间（来自 envelope），legacy cache 或无法获得时为 nil
	CreatedAt *time.Time
	// ExpiresAt 缓存过期时间（由 Redis TTL 推算：now + TTL），无法获得时为 nil
	ExpiresAt *time.Time
	// Legacy 是否为旧格式缓存（纯 Graph JSON，无 envelope）
	Legacy bool
}

// GetGraph 获取拓扑图，支持 Redis 缓存。
// refresh=true 时跳过缓存。
func (s *Service) GetGraph(ctx context.Context, cluster, namespace string, refresh bool) (*Graph, error) {
	graph, _, err := s.GetGraphWithProvenance(ctx, cluster, namespace, refresh)
	return graph, err
}

// GetGraphWithProvenance 获取拓扑图并返回缓存 provenance。
// provenance 真实描述数据来源（cache hit/miss, cacheCreatedAt, cacheExpiresAt）。
func (s *Service) GetGraphWithProvenance(ctx context.Context, cluster, namespace string, refresh bool) (*Graph, *CacheProvenance, error) {
	cacheKey := s.cacheKey(cluster, namespace)
	prov := &CacheProvenance{}

	// 尝试从缓存读取。
	if !refresh && s.rdb != nil {
		if data, err := s.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			// 优先尝试解析 envelope（新格式）。
			var env cacheEnvelope
			if json.Unmarshal(data, &env) == nil && env.Graph.Cluster != "" {
				prov.Hit = true
				createdAt := env.CreatedAt
				prov.CreatedAt = &createdAt
				// 从 Redis TTL 推算过期时间。
				if ttl, ttlErr := s.rdb.TTL(ctx, cacheKey).Result(); ttlErr == nil && ttl > 0 {
					expiresAt := time.Now().Add(ttl)
					prov.ExpiresAt = &expiresAt
				}
				s.enrichStatus(ctx, &env.Graph)
				return &env.Graph, prov, nil
			}
			// legacy cache：纯 Graph JSON。
			var graph Graph
			if json.Unmarshal(data, &graph) == nil {
				prov.Hit = true
				prov.Legacy = true
				// CreatedAt 无法获得，保持 nil（禁止伪造）。
				if ttl, ttlErr := s.rdb.TTL(ctx, cacheKey).Result(); ttlErr == nil && ttl > 0 {
					expiresAt := time.Now().Add(ttl)
					prov.ExpiresAt = &expiresAt
				}
				s.enrichStatus(ctx, &graph)
				return &graph, prov, nil
			}
		}
	}

	// 从 Kubernetes 构建（cache miss 或 refresh）。
	graph, err := s.builder.Build(ctx, cluster, namespace)
	if err != nil {
		return nil, prov, err
	}

	// 写入缓存（使用 envelope 包装，记录真实创建时间）。
	if s.rdb != nil {
		env := cacheEnvelope{Graph: *graph, CreatedAt: time.Now()}
		if data, err := json.Marshal(env); err == nil {
			if err := s.rdb.Set(ctx, cacheKey, data, s.cacheTTL).Err(); err != nil {
				slog.Warn("topology: cache set failed", "err", err)
			} else {
				createdAt := env.CreatedAt
				prov.CreatedAt = &createdAt
				expiresAt := time.Now().Add(s.cacheTTL)
				prov.ExpiresAt = &expiresAt
			}
		}
	}

	// enrich 状态。
	s.enrichStatus(ctx, graph)

	return graph, prov, nil
}

// enrichStatus 用 Incident/Alert/Anomaly 数据丰富节点状态。
func (s *Service) enrichStatus(ctx context.Context, graph *Graph) {
	if s.statusProvider == nil {
		return
	}
	for i := range graph.Nodes {
		status, incidentIDs, alertCount, anomalyCount := s.statusProvider.ResourceStatus(
			ctx, graph.Cluster, string(graph.Nodes[i].Type),
			graph.Nodes[i].Namespace, graph.Nodes[i].Name,
		)
		// 只有当外部状态比 K8s 自身状态更严重时才覆盖。
		if severityRank(status) > severityRank(graph.Nodes[i].Status) {
			graph.Nodes[i].Status = status
		}
		graph.Nodes[i].IncidentIDs = incidentIDs
		graph.Nodes[i].AlertCount = alertCount
		graph.Nodes[i].AnomalyCount = anomalyCount
	}
}

// severityRank 状态严重度排序。
func severityRank(s NodeStatus) int {
	switch s {
	case StatusCritical:
		return 3
	case StatusWarning:
		return 2
	case StatusHealthy:
		return 1
	default:
		return 0
	}
}

// GetNode 获取单个节点。
func (s *Service) GetNode(ctx context.Context, cluster string, typ ResourceType, namespace, name string) (*Node, error) {
	graph, err := s.GetGraph(ctx, cluster, namespace, false)
	if err != nil {
		return nil, err
	}
	id := NodeID(cluster, typ, namespace, name)
	for _, n := range graph.Nodes {
		if n.ID == id {
			return &n, nil
		}
	}
	return nil, fmt.Errorf("node not found: %s", id)
}

// GetDependencies 获取节点的依赖关系。
func (s *Service) GetDependencies(ctx context.Context, cluster string, typ ResourceType, namespace, name string) (*DependencyResult, error) {
	graph, err := s.GetGraph(ctx, cluster, namespace, false)
	if err != nil {
		return nil, err
	}

	id := NodeID(cluster, typ, namespace, name)
	nodeMap := make(map[string]Node)
	for _, n := range graph.Nodes {
		nodeMap[n.ID] = n
	}

	target, ok := nodeMap[id]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", id)
	}

	result := &DependencyResult{Node: target}

	// 直接下游（此节点 → 其他节点）。
	for _, e := range graph.Edges {
		if e.Source == id {
			if child, ok := nodeMap[e.Target]; ok {
				result.Children = append(result.Children, child)
			}
		}
		if e.Target == id {
			if parent, ok := nodeMap[e.Source]; ok {
				result.Parents = append(result.Parents, parent)
			}
		}
	}

	// 递归下游（所有后代）。
	visited := make(map[string]bool)
	result.Downstream = s.recursiveNeighbors(graph, id, true, visited)
	visited = make(map[string]bool)
	result.Upstream = s.recursiveNeighbors(graph, id, false, visited)

	return result, nil
}

// GetImpact 获取节点的影响范围（递归上游，即依赖此节点的所有节点）。
func (s *Service) GetImpact(ctx context.Context, cluster string, typ ResourceType, namespace, name string) (*ImpactResult, error) {
	graph, err := s.GetGraph(ctx, cluster, namespace, false)
	if err != nil {
		return nil, err
	}

	id := NodeID(cluster, typ, namespace, name)
	nodeMap := make(map[string]Node)
	for _, n := range graph.Nodes {
		nodeMap[n.ID] = n
	}

	target, ok := nodeMap[id]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", id)
	}

	visited := make(map[string]bool)
	affected := s.recursiveNeighbors(graph, id, false, visited)

	return &ImpactResult{
		Node:          target,
		AffectedNodes: affected,
		EdgeCount:     len(graph.Edges),
	}, nil
}

// recursiveNeighbors 递归获取邻居节点。
// downstream=true: 获取此节点指向的所有节点（后代）。
// downstream=false: 获取指向此节点的所有节点（上游/受影响）。
func (s *Service) recursiveNeighbors(graph *Graph, nodeID string, downstream bool, visited map[string]bool) []Node {
	var result []Node
	nodeMap := make(map[string]Node)
	for _, n := range graph.Nodes {
		nodeMap[n.ID] = n
	}

	var dfs func(id string)
	dfs = func(id string) {
		for _, e := range graph.Edges {
			var neighborID string
			if downstream && e.Source == id {
				neighborID = e.Target
			} else if !downstream && e.Target == id {
				neighborID = e.Source
			} else {
				continue
			}
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true
			if n, ok := nodeMap[neighborID]; ok {
				result = append(result, n)
			}
			dfs(neighborID)
		}
	}
	dfs(nodeID)
	return result
}

// InvalidateCache 清除指定集群的拓扑缓存。
func (s *Service) InvalidateCache(ctx context.Context, cluster, namespace string) {
	if s.rdb == nil {
		return
	}
	s.rdb.Del(ctx, s.cacheKey(cluster, namespace))
}
