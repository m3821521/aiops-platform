package incident

import "time"

// Incident 状态常量。
const (
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
	StatusClosed       = "closed"
)

// 严重级别，与 alert 包保持一致。
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// SignalType 是信号来源类型。
type SignalType string

const (
	SignalAlert    SignalType = "alert"
	SignalAnomaly  SignalType = "anomaly"
	SignalLog      SignalType = "log"
	SignalK8sEvent SignalType = "k8s_event"
	SignalMetric   SignalType = "metric"
)

// ResourceType 是关联资源类型。
type ResourceType string

const (
	ResourcePod        ResourceType = "pod"
	ResourceDeployment ResourceType = "deployment"
	ResourceService    ResourceType = "service"
	ResourceNode       ResourceType = "node"
	ResourceNamespace  ResourceType = "namespace"
	ResourceCluster    ResourceType = "cluster"
)

// Signal 是统一信号接口，所有来源（alert/anomaly/log/event/metric）都转换为此结构后参与关联。
type Signal struct {
	SignalType   SignalType        `json:"signal_type"`
	SignalID     string            `json:"signal_id"` // 外部唯一标识（alert fingerprint 等）
	Title        string            `json:"title"`     // 简短标题
	Severity     string            `json:"severity"`  // critical/warning/info
	Cluster      string            `json:"cluster"`
	Namespace    string            `json:"namespace"`
	Service      string            `json:"service"`
	ResourceType ResourceType      `json:"resource_type"`
	ResourceName string            `json:"resource_name"`
	Timestamp    time.Time         `json:"timestamp"`
	Resolved     bool              `json:"resolved"`
	Labels       map[string]string `json:"labels,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"` // 原始数据快照
}

// CorrelationScore 是关联评分的明细。
type CorrelationScore struct {
	TimeScore      float64 `json:"time_score"`
	ResourceScore  float64 `json:"resource_score"`
	NamespaceScore float64 `json:"namespace_score"`
	LabelScore     float64 `json:"label_score"`
	TopologyScore  float64 `json:"topology_score"`
	Total          float64 `json:"total"`
}

// CorrelationConfig 是关联引擎的可配置参数。
type CorrelationConfig struct {
	TimeWindow     time.Duration `json:"time_window"`   // 时间窗口，默认 ±5min
	ScoreThreshold float64       `json:"score_threshold"` // 关联阈值，默认 0.5
	// 各维度权重。
	WeightTime      float64 `json:"weight_time"`
	WeightResource  float64 `json:"weight_resource"`
	WeightNamespace float64 `json:"weight_namespace"`
	WeightLabel     float64 `json:"weight_label"`
	WeightTopology  float64 `json:"weight_topology"`
	// 资源匹配加分。
	ScoreSamePod        float64 `json:"score_same_pod"`
	ScoreSameDeployment float64 `json:"score_same_deployment"`
	ScoreSameService    float64 `json:"score_same_service"`
	ScoreSameNode       float64 `json:"score_same_node"`
	ScoreSameNamespace  float64 `json:"score_same_namespace"`
	// 标签匹配加分（匹配到一个 key 加一次）。
	ScoreLabelMatch float64 `json:"score_label_match"`
}

// DefaultCorrelationConfig 返回默认关联配置。
func DefaultCorrelationConfig() CorrelationConfig {
	return CorrelationConfig{
		TimeWindow:          5 * time.Minute,
		ScoreThreshold:      0.5,
		WeightTime:          0.2,
		WeightResource:      0.35,
		WeightNamespace:     0.15,
		WeightLabel:         0.15,
		WeightTopology:      0.15,
		ScoreSamePod:        1.0,
		ScoreSameDeployment: 0.9,
		ScoreSameService:    0.8,
		ScoreSameNode:       0.5,
		ScoreSameNamespace:  0.6,
		ScoreLabelMatch:     0.3,
	}
}

// TopologyCorrelationProvider 是拓扑关联提供者接口。
// P2-1 不实现，P2-3 Topology 模块完成后接入。
type TopologyCorrelationProvider interface {
	// Related 判断两个资源在拓扑上是否相邻（直接依赖或被依赖）。
	Related(cluster, ns1, resType1, resName1, resType2, resName2 string) bool
}
