package alert

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// 告警状态常量。
const (
	StatusFiring       = "firing"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
	StatusSuppressed   = "suppressed"
)

// 告警级别常量。
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// JSONMap 是 MySQL JSON 字段的 Go 映射，实现 GORM 的 Scanner/Valuer 接口。
// 用 map[string]string 而非 map[string]any，因为 Prometheus/Alertmanager 的 label 值都是字符串。
type JSONMap map[string]string

// Value 实现 driver.Valuer，序列化到数据库。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan 实现 sql.Scanner，从数据库反序列化。
func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = JSONMap{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("JSONMap Scan: expected []byte, got %T", value)
	}
	if len(bytes) == 0 {
		*m = JSONMap{}
		return nil
	}
	return json.Unmarshal(bytes, m)
}

// Alert 对应 alerts 表。
type Alert struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Fingerprint string         `gorm:"size:255;uniqueIndex" json:"fingerprint"`
	Alertname   string         `gorm:"size:255;index" json:"alertname"`
	Severity    string         `gorm:"size:32;index;default:warning" json:"severity"`
	Status      string         `gorm:"size:32;index;default:firing" json:"status"`
	Instance    string         `gorm:"size:255" json:"instance,omitempty"`
	Pod         string         `gorm:"size:255" json:"pod,omitempty"`
	Namespace   string         `gorm:"size:255" json:"namespace,omitempty"`
	Service     string         `gorm:"size:255" json:"service,omitempty"`
	Node        string         `gorm:"size:255" json:"node,omitempty"`
	Labels      JSONMap        `gorm:"type:json" json:"labels,omitempty"`
	Annotations JSONMap        `gorm:"type:json" json:"annotations,omitempty"`
	StartsAt    time.Time      `gorm:"index" json:"starts_at"`
	EndsAt      *time.Time     `json:"ends_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	// IncidentIDs 是非持久化字段，通过 incident_signals 表关联查询得到。
	// 关系：Alert.Fingerprint = IncidentSignal.SignalID (signal_type='alert')。
	// 一个 Alert 可能关联多个 Incident。
	IncidentIDs []int64 `gorm:"-" json:"incident_ids,omitempty"`
}

// TableName 指定表名。
func (Alert) TableName() string {
	return "alerts"
}
