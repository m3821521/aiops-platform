package anomaly

import (
	"time"

	"gorm.io/gorm"
)

// Anomaly 状态常量。
const (
	AnomalyStatusDetected = "detected" // 刚检测到
	AnomalyStatusActive   = "active"   // 活跃中
	AnomalyStatusResolved = "resolved" // 已恢复
)

// AnomalyRecord 对应 anomaly_records 表，持久化异常检测结果。
type AnomalyRecord struct {
	ID           int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Metric       string         `gorm:"size:255;index;not null" json:"metric"`
	ResourceType string         `gorm:"size:64;index" json:"resource_type,omitempty"`
	ResourceName string         `gorm:"size:255;index" json:"resource_name,omitempty"`
	Namespace    string         `gorm:"size:255;index" json:"namespace,omitempty"`
	Cluster      string         `gorm:"size:128;index" json:"cluster,omitempty"`
	Timestamp    time.Time      `gorm:"index;not null" json:"timestamp"`
	Value        float64        `json:"value"`
	Baseline     float64        `json:"baseline,omitempty"`
	ExpectedMin  float64        `json:"expected_min,omitempty"`
	ExpectedMax  float64        `json:"expected_max,omitempty"`
	AnomalyScore float64        `json:"anomaly_score"` // 0.0~1.0
	Severity     string         `gorm:"size:32;index;default:warning" json:"severity"`
	Algorithm    string         `gorm:"size:64;index" json:"algorithm"` // static_threshold/moving_average/ewma/z_score
	Reason       string         `gorm:"type:text" json:"reason,omitempty"`
	Status       string         `gorm:"size:32;index;default:detected" json:"status"`
	IncidentID   *int64         `gorm:"index" json:"incident_id,omitempty"`
	ResolvedAt   *time.Time     `json:"resolved_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AnomalyRecord) TableName() string { return "anomaly_records" }

// ToAnomaly 将持久化记录转换为检测结果结构。
func (r *AnomalyRecord) ToAnomaly() Anomaly {
	return Anomaly{
		Metric:       r.Metric,
		Timestamp:    r.Timestamp,
		Value:        r.Value,
		AnomalyScore: r.AnomalyScore,
		Severity:     r.Severity,
		Reason:       r.Reason,
		Detector:     r.Algorithm,
	}
}

// AnomalyRule 是异常检测规则配置（内存配置，第一版不持久化到数据库）。
type AnomalyRule struct {
	Name         string            `json:"name"`
	Metric       string            `json:"metric"`        // PromQL 查询或指标名
	ResourceType string            `json:"resource_type"` // pod/node/deployment/service
	Algorithm    string            `json:"algorithm"`     // static_threshold/moving_average/ewma/z_score
	Parameters   map[string]any    `json:"parameters"`    // 算法参数（阈值、窗口等）
	Severity     string            `json:"severity"`      // 默认严重度
	Enabled      bool              `json:"enabled"`
	Interval     time.Duration     `json:"interval"` // 检测间隔
	Namespace    string            `json:"namespace,omitempty"`
	Cluster      string            `json:"cluster,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// DefaultRules 返回默认检测规则（少量，覆盖核心资源）。
func DefaultRules() []AnomalyRule {
	return []AnomalyRule{
		{
			Name:         "node-cpu-high",
			Metric:       `100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`,
			ResourceType: "node",
			Algorithm:    "static_threshold",
			Parameters:   map[string]any{"upper_warning": 80.0, "upper_critical": 90.0},
			Severity:     "warning",
			Enabled:      true,
			Interval:     1 * time.Minute,
		},
		{
			Name:         "node-memory-high",
			Metric:       `(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100`,
			ResourceType: "node",
			Algorithm:    "static_threshold",
			Parameters:   map[string]any{"upper_warning": 85.0, "upper_critical": 95.0},
			Severity:     "warning",
			Enabled:      true,
			Interval:     1 * time.Minute,
		},
		{
			Name:         "pod-cpu-high",
			Metric:       `sum by (pod) (rate(container_cpu_usage_seconds_total{container!=""}[5m])) * 100`,
			ResourceType: "pod",
			Algorithm:    "static_threshold",
			Parameters:   map[string]any{"upper_warning": 80.0, "upper_critical": 95.0},
			Severity:     "warning",
			Enabled:      true,
			Interval:     1 * time.Minute,
		},
		{
			Name:         "pod-memory-high",
			Metric:       `sum by (pod) (container_memory_working_set_bytes{container!=""}) / sum by (pod) (container_spec_memory_limit_bytes{container!=""}) * 100`,
			ResourceType: "pod",
			Algorithm:    "static_threshold",
			Parameters:   map[string]any{"upper_warning": 85.0, "upper_critical": 95.0},
			Severity:     "warning",
			Enabled:      true,
			Interval:     1 * time.Minute,
		},
	}
}
