package rca

import "time"

// Severity 级别，与 alert 包保持一致。
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// AlertInfo 是 RCA 输入的告警信息（简化版，不依赖 alert 包避免循环引用）。
type AlertInfo struct {
	ID          int64             `json:"id"`
	Fingerprint string            `json:"fingerprint"`
	Alertname   string            `json:"alertname"`
	Severity    string            `json:"severity"`
	Service     string            `json:"service,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Pod         string            `json:"pod,omitempty"`
	Node        string            `json:"node,omitempty"`
	StartsAt    time.Time         `json:"starts_at"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// TimelineEvent 是时间线上的一个事件（旧版，保留兼容）。
type TimelineEvent struct {
	Time        time.Time `json:"time"`
	Service     string    `json:"service"`
	Alertname   string    `json:"alertname"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
}

// Result 是 RCA 分析结果。
type Result struct {
	RootCause        string          `json:"root_cause"`
	Confidence       float64         `json:"confidence"` // 0.0 ~ 1.0
	AffectedServices []string        `json:"affected_services"`
	Evidence         []Evidence      `json:"evidence"`
	Timeline         []TimelineEvent `json:"timeline"`
	AnalyzedAt       time.Time       `json:"analyzed_at"`
}

// serviceStats 是单个服务的告警统计。
type serviceStats struct {
	Name      string
	Namespace string
	Count     int
	Critical  int
	Warning   int
	Alerts    []AlertInfo
	FirstSeen time.Time
	LastSeen  time.Time
}

// EvidenceBundle 是统一 Evidence 收集结果，用于 GET /incidents/:id/evidence API。
type EvidenceBundle struct {
	IncidentID   int64              `json:"incident_id"`
	Cluster      string             `json:"cluster,omitempty"`
	Namespace    string             `json:"namespace,omitempty"`
	Service      string             `json:"service,omitempty"`
	ResourceType string             `json:"resource_type,omitempty"`
	ResourceName string             `json:"resource_name,omitempty"`
	TimeWindow   EvidenceTimeWindow `json:"time_window"`
	Sources      EvidenceSources    `json:"sources"`
	Alerts       []AlertInfo        `json:"alerts"`
	Anomalies    []AnomalyInfo      `json:"anomalies"`
	Events       []EventInfo        `json:"events"`
	Metrics      []MetricInfo       `json:"metrics"`
	Logs         []LogInfo          `json:"logs"`
	Topology     TopologyInfo       `json:"topology"`
	Timeline     []TimelineItem     `json:"timeline"`
	PodResourceState *PodResourceState `json:"pod_resource_state,omitempty"`
}

type EvidenceTimeWindow struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Before time.Time `json:"before"`
}

type EvidenceSources struct {
	Alerts    string `json:"alerts"`
	Anomalies string `json:"anomalies"`
	Events    string `json:"events"`
	Metrics   string `json:"metrics"`
	Logs      string `json:"logs"`
	Topology  string `json:"topology"`
	PodResourceState string `json:"pod_resource_state"`
}

// PodResourceState 是 Kubernetes Pod 的实时资源状态证据。
type PodResourceState struct {
	Namespace   string                  `json:"namespace"`
	Pod         string                  `json:"pod"`
	Phase       string                  `json:"phase"`
	Ready       bool                    `json:"ready"`
	RestartCount int32                 `json:"restart_count"`
	NodeName    string                  `json:"node_name,omitempty"`
	PodIP       string                  `json:"pod_ip,omitempty"`
	HostIP      string                  `json:"host_ip,omitempty"`
	StartTime   string                  `json:"start_time,omitempty"`
	Containers  []PodContainerState     `json:"containers"`
	Conditions  []PodCondition          `json:"conditions"`
}

type PodContainerState struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"` // running/waiting/terminated/unknown
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	ExitCode     *int32 `json:"exit_code,omitempty"`
	Signal       *int32 `json:"signal,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
	LastState    string `json:"last_state,omitempty"`
	LastReason   string `json:"last_reason,omitempty"`
	LastExitCode *int32 `json:"last_exit_code,omitempty"`
}

type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
