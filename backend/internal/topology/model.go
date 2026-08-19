package topology

// ResourceType 是拓扑节点的资源类型。
type ResourceType string

const (
	TypeNode       ResourceType = "node"
	TypePod        ResourceType = "pod"
	TypeDeployment ResourceType = "deployment"
	TypeService    ResourceType = "service"
	TypeIngress    ResourceType = "ingress"
)

// RelationType 是拓扑边的关系类型。
type RelationType string

const (
	RelationOwns    RelationType = "owns"      // Deployment → Pod
	RelationRunsOn  RelationType = "runs_on"   // Pod → Node
	RelationSelects RelationType = "selects"   // Service → Pod
	RelationRoutes  RelationType = "routes_to" // Ingress → Service
)

// NodeStatus 是节点状态。
type NodeStatus string

const (
	StatusHealthy  NodeStatus = "healthy"
	StatusWarning  NodeStatus = "warning"
	StatusCritical NodeStatus = "critical"
	StatusUnknown  NodeStatus = "unknown"
)

// Node 是拓扑图中的一个节点（Kubernetes 资源）。
type Node struct {
	ID           string            `json:"id"` // cluster/type/namespace/name
	Type         ResourceType      `json:"type"`
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace,omitempty"`
	Cluster      string            `json:"cluster"`
	Status       NodeStatus        `json:"status"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"` // 额外信息（如 pod IP, replicas 等）
	IncidentIDs  []int64           `json:"incident_ids,omitempty"`
	AlertCount   int               `json:"alert_count,omitempty"`
	AnomalyCount int               `json:"anomaly_count,omitempty"`
}

// Edge 是拓扑图中的一条边（资源关系）。
type Edge struct {
	ID       string       `json:"id"` // source + relation + target
	Source   string       `json:"source"`
	Target   string       `json:"target"`
	Relation RelationType `json:"relation"`
	Cluster  string       `json:"cluster,omitempty"`
}

// Graph 是完整的拓扑图。
type Graph struct {
	Cluster string `json:"cluster"`
	Nodes   []Node `json:"nodes"`
	Edges   []Edge `json:"edges"`
}

// NodeID 生成稳定的节点 ID。
func NodeID(cluster string, typ ResourceType, namespace, name string) string {
	if namespace == "" {
		return cluster + "/" + string(typ) + "/" + name
	}
	return cluster + "/" + string(typ) + "/" + namespace + "/" + name
}

// EdgeID 生成稳定的边 ID。
func EdgeID(source, target string, relation RelationType) string {
	return source + "|" + string(relation) + "|" + target
}

// DependencyResult 是依赖分析结果。
type DependencyResult struct {
	Node       Node   `json:"node"`
	Upstream   []Node `json:"upstream"`   // 依赖此节点的节点（反向）
	Downstream []Node `json:"downstream"` // 此节点依赖的节点
	Parents    []Node `json:"parents"`    // 直接上游
	Children   []Node `json:"children"`   // 直接下游
}

// ImpactResult 是影响分析结果。
type ImpactResult struct {
	Node          Node   `json:"node"`
	AffectedNodes []Node `json:"affected_nodes"` // 受影响的所有节点（递归上游）
	EdgeCount     int    `json:"edge_count"`
}
