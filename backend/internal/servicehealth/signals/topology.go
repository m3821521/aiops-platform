package signals

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/topology"
)

// TopologyGraphGetter 是 Topology 图获取接口。
// 真实实现为 *topology.Service。定义在 signals 包内便于测试 mock。
type TopologyGraphGetter interface {
	GetGraph(ctx context.Context, cluster, namespace string, refresh bool) (*topology.Graph, error)
}

// TopologySignalCollector 从 Topology 采集 Service Health Signals。
//
// 支持的 Signal 类型：
//   - dependency_health: Service 节点及其依赖的健康状态
//
// 原则：
//   - Topology 没有独立的 DataTimestamp，使用 fetchedAt 作为 dataTimestamp。
//   - TimestampAvailable=false（拓扑状态是计算结果，不是原始事件时间）。
//   - 不伪造业务事件时间。
type TopologySignalCollector struct {
	topology TopologyGraphGetter
}

// NewTopologySignalCollector 创建 TopologySignalCollector。
func NewTopologySignalCollector(topo TopologyGraphGetter) *TopologySignalCollector {
	return &TopologySignalCollector{topology: topo}
}

// Source 实现 SignalCollector 接口。
func (c *TopologySignalCollector) Source() string { return "topology" }

// Collect 实现 SignalCollector 接口。
// 获取 Topology 图，找到 Service 节点及其依赖，转换为 dependency_health signal。
func (c *TopologySignalCollector) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error) {
	if c.topology == nil {
		return nil, nil
	}

	fetchedAt := time.Now()

	// 获取 Topology 图（使用缓存，不强制 refresh）
	graph, err := c.topology.GetGraph(ctx, svc.Cluster, svc.Namespace, false)
	if err != nil {
		return nil, fmt.Errorf("topology get graph failed: %w", err)
	}
	if graph == nil {
		return nil, nil
	}

	// 找到 Service 节点
	serviceNodeID := topology.NodeID(svc.Cluster, topology.TypeService, svc.Namespace, svc.Name)
	var serviceNode *topology.Node
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == serviceNodeID {
			serviceNode = &graph.Nodes[i]
			break
		}
	}
	if serviceNode == nil {
		// Service 不在拓扑图中 → empty
		return nil, nil
	}

	// 构建节点 ID → Node 映射
	nodeMap := make(map[string]*topology.Node, len(graph.Nodes))
	for i := range graph.Nodes {
		nodeMap[graph.Nodes[i].ID] = &graph.Nodes[i]
	}

	// 找到 Service 的直接依赖（children：Service → Pod → Deployment 等）
	var dependencies []topology.Node
	for _, edge := range graph.Edges {
		if edge.Source == serviceNodeID {
			if child, ok := nodeMap[edge.Target]; ok {
				dependencies = append(dependencies, *child)
			}
		}
	}

	// 判断整体依赖健康状态
	overallStatus := topology.StatusHealthy
	hasWarning := false
	hasCritical := false
	for _, dep := range dependencies {
		switch dep.Status {
		case topology.StatusCritical:
			hasCritical = true
		case topology.StatusWarning:
			hasWarning = true
		}
	}
	if hasCritical {
		overallStatus = topology.StatusCritical
	} else if hasWarning {
		overallStatus = topology.StatusWarning
	}

	// 如果 Service 自身状态更严重，使用 Service 自身状态
	if severityRank(serviceNode.Status) > severityRank(overallStatus) {
		overallStatus = serviceNode.Status
	}

	// 如果全部 healthy，不产生 signal（empty 不是 error）
	if overallStatus == topology.StatusHealthy || overallStatus == topology.StatusUnknown {
		return nil, nil
	}

	// 产生 dependency_health signal
	level := rca.EvidenceLevelContext
	causal := "contextual"
	score := 0.1
	if overallStatus == topology.StatusCritical {
		level = rca.EvidenceLevelCorroborating
		causal = "supporting"
		score = 0.4
	} else if overallStatus == topology.StatusWarning {
		level = rca.EvidenceLevelCorroborating
		causal = "supporting"
		score = 0.25
	}

	return []rca.Evidence{{
		ID:            fmt.Sprintf("dependency-health-%s-%s-%s-%d", svc.Cluster, svc.Namespace, svc.Name, fetchedAt.Unix()),
		Type:          "dependency_health",
		Level:         level,
		Source:        "topology",
		SourceType:    "provider",
		Timestamp:     fetchedAt,
		ResourceType:  "service",
		ResourceName:  svc.Name,
		Namespace:     svc.Namespace,
		Description:   fmt.Sprintf("Service %s dependency health: %s (alerts=%d, anomalies=%d, dependencies=%d)", svc.Name, overallStatus, serviceNode.AlertCount, serviceNode.AnomalyCount, len(dependencies)),
		Score:         score,
		FetchedAt:     &fetchedAt,
		DataTimestamp: &fetchedAt,
		TimestampAvailable: false, // 拓扑状态是计算结果，没有原始事件时间
		TrustStatus:        "fresh",
		CausalRelevance:    causal,
	}}, nil
}

// severityRank 拓扑状态严重度排序（与 topology/service.go 一致）。
func severityRank(s topology.NodeStatus) int {
	switch s {
	case topology.StatusCritical:
		return 3
	case topology.StatusWarning:
		return 2
	case topology.StatusHealthy:
		return 1
	default:
		return 0
	}
}
