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
	ID          int64
	Fingerprint string
	Alertname   string
	Severity    string
	Service     string
	Namespace   string
	Pod         string
	Node        string
	StartsAt    time.Time
	Labels      map[string]string
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
