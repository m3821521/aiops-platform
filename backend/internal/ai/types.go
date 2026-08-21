package ai

import "time"

// DataSourceStatus 是数据源状态。
type DataSourceStatus struct {
	AlertsAvailable    bool `json:"alerts_available"`
	AnomaliesAvailable bool `json:"anomalies_available"`
	MetricsAvailable   bool `json:"metrics_available"`
	LogsAvailable      bool `json:"logs_available"`
	EventsAvailable    bool `json:"events_available"`
	TopologyAvailable  bool `json:"topology_available"`
	RcaAvailable       bool `json:"rca_available"`
}

// AIContext 是 AI 分析的输入上下文。
// 所有数据必须来自真实数据源，禁止编造。
type AIContext struct {
	IncidentID   int64              `json:"incident_id"`
	Cluster      string             `json:"cluster"`
	Namespace    string             `json:"namespace"`
	Service      string             `json:"service"`
	ResourceType string             `json:"resource_type"`
	ResourceName string             `json:"resource_name"`
	StartTime    time.Time          `json:"start_time"`
	EndTime      time.Time          `json:"end_time"`
	IncidentTitle string            `json:"incident_title"`
	IncidentSeverity string         `json:"incident_severity"`
	RCA          *RCASummary        `json:"rca,omitempty"`
	Alerts       []AlertSummary     `json:"alerts,omitempty"`
	Anomalies    []AnomalySummary   `json:"anomalies,omitempty"`
	Metrics      []MetricSummary    `json:"metrics,omitempty"`
	Logs         []LogSummary       `json:"logs,omitempty"`
	Events       []EventSummary     `json:"events,omitempty"`
	Topology     *TopologySummary       `json:"topology,omitempty"`
	PodDiagnostic        *PodDiagnosticSummary        `json:"pod_diagnostic,omitempty"`
	DeploymentDiagnostic *DeploymentDiagnosticSummary `json:"deployment_diagnostic,omitempty"`
	ServiceDiagnostic    *ServiceDiagnosticSummary    `json:"service_diagnostic,omitempty"`
	DataSources          DataSourceStatus              `json:"data_sources"`
}

// RCASummary 是 RCA 结果摘要。
type RCASummary struct {
	RootCause  string  `json:"root_cause"`
	Confidence float64 `json:"confidence"`
	Status     string  `json:"status"`
	EvidenceCount int  `json:"evidence_count"`
}

// AlertSummary 是告警摘要。
type AlertSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Severity  string    `json:"severity"`
	Service   string    `json:"service,omitempty"`
	Pod       string    `json:"pod,omitempty"`
	Node      string    `json:"node,omitempty"`
	Namespace string    `json:"namespace,omitempty"`
	StartsAt  time.Time `json:"starts_at"`
}

// AnomalySummary 是异常摘要。
type AnomalySummary struct {
	ID           string    `json:"id"`
	Metric       string    `json:"metric"`
	ResourceType string    `json:"resource_type"`
	ResourceName string    `json:"resource_name"`
	Value        float64   `json:"value"`
	Baseline     float64   `json:"baseline"`
	Score        float64   `json:"score"`
	Severity     string    `json:"severity"`
	Algorithm    string    `json:"algorithm"`
	Reason       string    `json:"reason"`
	Timestamp    time.Time `json:"timestamp"`
}

// MetricSummary 是指标摘要。
type MetricSummary struct {
	ID        string    `json:"id"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Resource  string    `json:"resource"`
	Timestamp time.Time `json:"timestamp"`
}

// LogSummary 是日志摘要。
type LogSummary struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Pod       string    `json:"pod,omitempty"`
	Namespace string    `json:"namespace,omitempty"`
}

// EventSummary 是 Kubernetes Event 摘要。
type EventSummary struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"` // Normal/Warning
	Reason       string    `json:"reason"`
	Message      string    `json:"message"`
	ResourceType string    `json:"resource_type"`
	ResourceName string    `json:"resource_name"`
	Namespace    string    `json:"namespace,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Count        int32     `json:"count"`
}

// PodDiagnosticSummary 是 Pod 诊断信息摘要（用于 AI RCA）。
type PodDiagnosticSummary struct {
	ID           string                   `json:"id"`
	Namespace    string                   `json:"namespace"`
	Pod          string                   `json:"pod"`
	PodUID       string                   `json:"pod_uid,omitempty"` // Pod UID，用于 Historical State 关联
	Phase        string                   `json:"phase"`
	Ready        bool                     `json:"ready"`
	RestartCount int32                    `json:"restart_count"`
	NodeName     string                   `json:"node_name,omitempty"`
	StartTime    string                   `json:"start_time,omitempty"`
	Containers   []PodContainerDiagnostic `json:"containers"`
}

// PodContainerDiagnostic 是容器诊断信息摘要。
type PodContainerDiagnostic struct {
	Name           string `json:"name"`
	Ready          bool   `json:"ready"`
	RestartCount   int32  `json:"restart_count"`
	State          string `json:"state"` // running/waiting/terminated/unknown
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
	ExitCode       *int32 `json:"exit_code,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	FinishedAt     string `json:"finished_at,omitempty"`
	LastState      string `json:"last_state,omitempty"`
	LastReason     string `json:"last_reason,omitempty"`
	LastExitCode   *int32 `json:"last_exit_code,omitempty"`
	LastStartedAt  string `json:"last_started_at,omitempty"`
	LastFinishedAt string `json:"last_finished_at,omitempty"`
}

// DeploymentDiagnosticSummary 是 Deployment 诊断信息摘要（用于 AI RCA）。
type DeploymentDiagnosticSummary struct {
	ID                string `json:"id"`
	Namespace         string `json:"namespace"`
	Deployment        string `json:"deployment"`
	Replicas          int32  `json:"replicas"`           // desired replicas
	AvailableReplicas int32  `json:"available_replicas"` // available replicas
	ReadyReplicas     int32  `json:"ready_replicas"`     // ready replicas
	UpdatedReplicas   int32  `json:"updated_replicas"`   // updated replicas
	UnavailableReplicas int32 `json:"unavailable_replicas"` // unavailable replicas
	Condition         string `json:"condition,omitempty"` // Available/Progressing condition
	ConditionReason   string `json:"condition_reason,omitempty"`
	ConditionMessage  string `json:"condition_message,omitempty"`
}

// ServiceDiagnosticSummary 是 Service 诊断信息摘要（用于 AI RCA）。
type ServiceDiagnosticSummary struct {
	ID                   string            `json:"id"`
	Namespace            string            `json:"namespace"`
	ServiceName          string            `json:"service_name"`
	ServiceType          string            `json:"service_type,omitempty"`
	ClusterIP            string            `json:"cluster_ip,omitempty"`
	Ports                []ServicePortInfo `json:"ports,omitempty"`
	Selector             map[string]string `json:"selector,omitempty"`
	SelectorMatchStatus  string            `json:"selector_match_status"`  // matched/no_pods_matched/no_ready_pods
	MatchedPodCount      int32             `json:"matched_pod_count"`
	ReadyMatchedPodCount int32             `json:"ready_matched_pod_count"`
	EndpointCount        int32             `json:"endpoint_count"`
	ReadyEndpointCount   int32             `json:"ready_endpoint_count"`
	EndpointAddresses    []string          `json:"endpoint_addresses,omitempty"`
}

// ServicePortInfo 是 Service 端口信息。
type ServicePortInfo struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// TopologySummary 是拓扑摘要（只保留 Incident 相关子图）。
type TopologySummary struct {
	NodeCount  int                 `json:"node_count"`
	EdgeCount  int                 `json:"edge_count"`
	Nodes      []TopologyNodeInfo  `json:"nodes,omitempty"`
	Edges      []TopologyEdgeInfo  `json:"edges,omitempty"`
}

type TopologyNodeInfo struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status"`
}

type TopologyEdgeInfo struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

// AIEvidence 是 AI 输出的证据（必须来自 Context，禁止编造）。
type AIEvidence struct {
	ID          string `json:"id"` // 必须匹配 Context 中的 Evidence ID
	Type        string `json:"type"` // alert/anomaly/metric/log/event/topology
	Source      string `json:"source"`
	Resource    string `json:"resource,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
	Description string `json:"description"`
	Importance  string `json:"importance"` // high/medium/low
}

// ImpactItem 是影响范围项。
type ImpactItem struct {
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
	Namespace    string `json:"namespace,omitempty"`
	ImpactLevel  string `json:"impact_level"` // critical/high/medium/low
}

// Recommendation 是 AI 建议。
type Recommendation struct {
	Priority    string                 `json:"priority"`    // P0/P1/P2/P3
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Reason      string                 `json:"reason"`
	Risk        string                 `json:"risk"`        // low/medium/high/critical
	ActionType  string                 `json:"action_type"` // restart_pod/scale_deployment/jenkins_build/argocd_sync
	Target      string                 `json:"target"`      // 目标资源，如 pod名称/deployment名称
	Namespace   string                 `json:"namespace"`   // 命名空间
	Parameters  map[string]interface{} `json:"parameters"`  // 操作参数，如 replicas: 3
}

// Risk 是风险评估。
type Risk struct {
	Level       string `json:"level"` // low/medium/high/critical
	Description string `json:"description"`
}

// Action 是下一步操作建议。
type Action struct {
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

// AIAnalysisResult 是结构化 AI 分析结果。
type AIAnalysisResult struct {
	Summary             string           `json:"summary"`
	RootCauseExplanation string          `json:"root_cause_explanation"`
	Confidence          float64          `json:"confidence"` // 0.0~1.0
	Evidence            []AIEvidence     `json:"evidence"`
	Impact              []ImpactItem     `json:"impact"`
	Recommendations     []Recommendation `json:"recommendations"`
	Risks               []Risk           `json:"risks"`
	NextActions         []Action         `json:"next_actions"`
	DataSources         DataSourceStatus `json:"data_sources"`
	GeneratedAt         time.Time        `json:"generated_at"`
	Model               string           `json:"model,omitempty"`
	CreatedActions      []CreatedAction  `json:"created_actions,omitempty"` // AI 自动创建的 Action
}
