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
	EvidenceTypePodStatus EvidenceType = "pod_status" // P1-X.10 Phase 6: Pod container status (lastState.terminated)
)

// EvidenceLevel 是证据分级（P1-X.10 Evidence-First）。
type EvidenceLevel string

const (
	// EvidenceLevelDirect 可以直接证明故障原因：OOMKilled、ExitCode、previous logs error、PVC mount failed 等。
	EvidenceLevelDirect EvidenceLevel = "direct"
	// EvidenceLevelCorroborating 可以增强某个假设但非最终证据：CrashLoopBackOff、RestartCount 递增、Ready=False 等。
	EvidenceLevelCorroborating EvidenceLevel = "corroborating"
	// EvidenceLevelContext 仅作背景：Alert 名称、severity、resource name 等，禁止单独用于确认 Root Cause。
	EvidenceLevelContext EvidenceLevel = "context"
)

// RootCauseStatus 是根因确认状态（P1-X.10 Evidence-First）。
type RootCauseStatus string

const (
	// RootCauseStatusUnknown 无足够证据，无法确定根因。
	RootCauseStatusUnknown RootCauseStatus = "unknown"
	// RootCauseStatusHypothesis 只有 Alert/Metric 等 Context 证据，存在可能原因但无直接证据。
	RootCauseStatusHypothesis RootCauseStatus = "hypothesis"
	// RootCauseStatusProbable 存在多个 Corroborating Evidence，但仍缺少最终 Direct Evidence。
	RootCauseStatusProbable RootCauseStatus = "probable"
	// RootCauseStatusConfirmed 存在明确、可验证的 Direct Evidence 链，可以确认根因。
	RootCauseStatusConfirmed RootCauseStatus = "confirmed"
)

// RecommendationType 是建议操作类型（P1-X.10 Safety Gate）。
type RecommendationType string

const (
	// RecommendationTypeInvestigation 调查操作：收集日志、描述 Pod、收集事件、检查配置等，无副作用。
	RecommendationTypeInvestigation RecommendationType = "investigation"
	// RecommendationTypeRemediation 修复操作：重启 Pod、同步 ArgoCD、扩容 Deployment 等，有副作用，仅在 root_cause_status=confirmed 时允许。
	RecommendationTypeRemediation RecommendationType = "remediation"
	// RecommendationTypeVerification 验证操作：验证 Pod Ready、验证告警恢复、验证 Prometheus target 等。
	RecommendationTypeVerification RecommendationType = "verification"
)

// Recommendation 是 RCA 生成的建议操作。
type Recommendation struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Type          RecommendationType `json:"type"`
	Action        string           `json:"action"`
	Reason        string           `json:"reason"`
	Risk          string           `json:"risk"`
	EvidenceRefs  []string         `json:"evidence_refs,omitempty"`
	Allowed       bool             `json:"allowed"` // Safety Gate：未确认根因时 Remediation 操作 allowed=false
}

// PossibleCause 是可能原因（当 root_cause_status != confirmed 时输出）。
type PossibleCause struct {
	Cause       string         `json:"cause"`
	Status      RootCauseStatus `json:"status"`
	Confidence  float64        `json:"confidence"`
	EvidenceIDs []string       `json:"evidence_ids,omitempty"`
}

// EvidenceSufficiency 是证据充足性评估（P1-X.10 Sufficiency Gate）。
type EvidenceSufficiency struct {
	Sufficient              bool     `json:"sufficient"`
	DirectEvidenceCount     int      `json:"direct_evidence_count"`
	CorroboratingCount      int      `json:"corroborating_evidence_count"`
	ContextCount            int      `json:"context_count"`
	MissingEvidence         []string `json:"missing_evidence,omitempty"`
	ConfidenceCap           float64  `json:"confidence_cap"`
	ConfidenceCapReason     string   `json:"confidence_cap_reason,omitempty"`
}

// Evidence 是 RCA 的一条统一证据。
type Evidence struct {
	ID            string        `json:"id"`
	Order         int           `json:"order,omitempty"` // 兼容旧 Engine
	Type          EvidenceType  `json:"type"`
	Level         EvidenceLevel `json:"level,omitempty"` // P1-X.10: direct/corroborating/context
	Source        string        `json:"source"`
	SourceType    string        `json:"sourceType,omitempty"` // P1-X.10: provider/cache/database
	Timestamp     time.Time     `json:"timestamp"`
	ResourceType  string        `json:"resource_type,omitempty"`
	ResourceName  string        `json:"resource_name,omitempty"`
	Namespace     string        `json:"namespace,omitempty"`
	Metric        string        `json:"metric,omitempty"`
	Value         float64       `json:"value,omitempty"`
	Expected      string        `json:"expected,omitempty"`
	Severity      string        `json:"severity,omitempty"`
	Description   string        `json:"description"`
	Score         float64       `json:"score"` // 0.0~1.0，这条证据对根因的支持程度
	RelatedSignal string        `json:"related_signal,omitempty"`

	// P1-X.10 Phase 4: Evidence 级 Provenance
	FetchedAt          *time.Time `json:"fetchedAt,omitempty"`          // 后端获取该证据的时间
	DataTimestamp      *time.Time `json:"dataTimestamp,omitempty"`      // 数据本身的时间（如 Prometheus sample timestamp、K8s event timestamp）
	TimestampAvailable bool       `json:"timestampAvailable,omitempty"` // 是否有真实 dataTimestamp
	TrustStatus        string     `json:"trustStatus,omitempty"`        // fresh/stale/error/empty
	CausalRelevance    string     `json:"causalRelevance,omitempty"`    // direct_causal/supporting/contextual/contradictory
}

// RootCauseCandidate 是一个候选根因。
type RootCauseCandidate struct {
	ResourceType string           `json:"resource_type"`
	ResourceName string           `json:"resource_name"`
	Namespace    string           `json:"namespace,omitempty"`
	RootCause    string           `json:"root_cause"`
	Status       RootCauseStatus  `json:"status,omitempty"` // P1-X.10: unknown/hypothesis/probable/confirmed
	Score        float64          `json:"score"`        // 排序分数 0.0~1.0
	Confidence   float64          `json:"confidence"`   // 置信度 0.0~1.0
	Evidence     []Evidence       `json:"evidence"`
	Impact       []string         `json:"impact,omitempty"`
	Explanation  string           `json:"explanation"`
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

	// P1-X.10 DataTrust: 记录每个数据源的采集错误，用于区分 Empty vs Error。
	SourceErrors map[string]error `json:"-"`
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
	IncidentID        int64                `json:"incident_id"`
	Status            RCAStatus            `json:"status"`
	RootCause         string               `json:"root_cause"`
	RootCauseStatus   RootCauseStatus      `json:"root_cause_status,omitempty"` // P1-X.10: unknown/hypothesis/probable/confirmed
	Confidence        float64              `json:"confidence"`
	ConfidenceReason  string               `json:"confidence_reason,omitempty"`  // P1-X.10: 置信度推导说明
	EvidenceSufficiency *EvidenceSufficiency `json:"evidence_sufficiency,omitempty"` // P1-X.10: 证据充足性评估
	PossibleCauses    []PossibleCause      `json:"possible_causes,omitempty"`    // P1-X.10: 可能原因（未确认时输出）
	Recommendations   []Recommendation     `json:"recommendations,omitempty"`     // P1-X.10: 建议操作（含 Safety Gate）
	Candidates        []RootCauseCandidate `json:"candidates"`
	Evidence          []Evidence           `json:"evidence"`
	Impact            []string             `json:"impact"`
	Timeline          []TimelineItem       `json:"timeline"`
	Explanation       string               `json:"explanation"`
	GeneratedAt       time.Time            `json:"generated_at"`
}
