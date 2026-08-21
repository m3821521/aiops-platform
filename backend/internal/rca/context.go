package rca

import "time"

// RCAStatus 是 RCA 分析状态。
type RCAStatus string

const (
	RCAStatusAnalyzing           RCAStatus = "analyzing"
	RCAStatusCompleted           RCAStatus = "completed"
	RCAStatusInsufficientEvidence RCAStatus = "insufficient_evidence"
	RCAStatusFailed              RCAStatus = "failed"
)

// EvidenceType 是证据类型。
type EvidenceType string

const (
	EvidenceTypeAlert    EvidenceType = "alert"
	EvidenceTypeAnomaly  EvidenceType = "anomaly"
	EvidenceTypeMetric   EvidenceType = "metric"
	EvidenceTypeLog      EvidenceType = "log"
	EvidenceTypeEvent    EvidenceType = "event"
	EvidenceTypeTopology EvidenceType = "topology"
)

// Evidence 是 RCA 的一条统一证据。
type Evidence struct {
	ID           string       `json:"id"`
	Order        int          `json:"order,omitempty"` // 兼容旧 Engine
	Type         EvidenceType `json:"type"`
	Source       string       `json:"source"`
	Timestamp    time.Time    `json:"timestamp"`
	ResourceType string       `json:"resource_type,omitempty"`
	ResourceName string       `json:"resource_name,omitempty"`
	Namespace    string       `json:"namespace,omitempty"`
	Metric       string       `json:"metric,omitempty"`
	Value        float64      `json:"value,omitempty"`
	Expected     string       `json:"expected,omitempty"`
	Severity     string       `json:"severity,omitempty"`
	Description  string       `json:"description"`
	Score        float64      `json:"score"` // 0.0~1.0，这条证据对根因的支持程度
	RelatedSignal string      `json:"related_signal,omitempty"`
}

// RootCauseCandidate 是一个候选根因。
type RootCauseCandidate struct {
	ResourceType string     `json:"resource_type"`
	ResourceName string     `json:"resource_name"`
	Namespace    string     `json:"namespace,omitempty"`
	RootCause    string     `json:"root_cause"`
	Score        float64    `json:"score"`        // 排序分数 0.0~1.0
	Confidence   float64    `json:"confidence"`   // 置信度 0.0~1.0
	Evidence     []Evidence `json:"evidence"`
	Impact       []string   `json:"impact,omitempty"`
	Explanation  string     `json:"explanation"`
}

// TimelineItem 是 RCA 时间线上的一个事件。
type TimelineItem struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"` // alert/anomaly/event/metric/log
	Description string    `json:"description"`
	Severity    string    `json:"severity,omitempty"`
	Resource    string    `json:"resource,omitempty"`
}

// IncidentContext 是 RCA 的输入上下文。
type IncidentContext struct {
	IncidentID   int64
	Cluster      string
	Namespace    string
	Service      string
	ResourceType string
	ResourceName string
	StartTime    time.Time
	EndTime      time.Time

	Alerts    []AlertInfo
	Anomalies []AnomalyInfo
	Events    []EventInfo
	Metrics   []MetricInfo
	Logs      []LogInfo
	Topology  TopologyInfo
	PodResourceState *PodResourceState
}

// AnomalyInfo 是异常信息（简化版，不依赖 anomaly 包避免循环引用）。
type AnomalyInfo struct {
	ID           int64   `json:"id"`
	Metric       string  `json:"metric"`
	ResourceType string  `json:"resource_type"`
	ResourceName string  `json:"resource_name"`
	Namespace    string  `json:"namespace,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Value        float64 `json:"value"`
	Baseline     float64 `json:"baseline"`
	AnomalyScore float64 `json:"anomaly_score"`
	Severity     string  `json:"severity"`
	Algorithm    string  `json:"algorithm,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

// EventInfo 是 Kubernetes Event 信息。
type EventInfo struct {
	Type         string    `json:"type"` // Normal/Warning
	Reason       string    `json:"reason"` // OOMKilled, FailedScheduling 等
	Message      string    `json:"message"`
	ResourceType string    `json:"resource_type"`
	ResourceName string    `json:"resource_name"`
	Namespace    string    `json:"namespace,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Count        int32     `json:"count"`
}

// MetricInfo 是指标信息。
type MetricInfo struct {
	Metric    string            `json:"metric"`
	Value     float64           `json:"value"`
	Timestamp time.Time         `json:"timestamp"`
	Resource  string            `json:"resource"`
	Unit      string            `json:"unit,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	// 时间序列数据（Before/During/After）
	Before []MetricDataPoint `json:"before,omitempty"`
	During []MetricDataPoint `json:"during,omitempty"`
	After  []MetricDataPoint `json:"after,omitempty"`
}

// MetricDataPoint 是指标时间序列数据点。
type MetricDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// LogInfo 是日志信息。
type LogInfo struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Pod       string    `json:"pod"`
	Namespace string    `json:"namespace"`
}

// TopologyInfo 是拓扑信息（简化版）。
type TopologyInfo struct {
	Nodes []TopologyNodeInfo `json:"nodes"`
	Edges []TopologyEdgeInfo `json:"edges"`
}

type TopologyNodeInfo struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
}

type TopologyEdgeInfo struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

// RCAResult 是 RCA 分析结果（新版）。
type RCAResult struct {
	IncidentID   int64               `json:"incident_id"`
	Status       RCAStatus           `json:"status"`
	RootCause    string              `json:"root_cause"`
	Confidence   float64             `json:"confidence"`
	Candidates   []RootCauseCandidate `json:"candidates"`
	Evidence     []Evidence          `json:"evidence"`
	Impact       []string            `json:"impact"`
	Timeline     []TimelineItem      `json:"timeline"`
	Explanation  string              `json:"explanation"`
	GeneratedAt  time.Time           `json:"generated_at"`
}
