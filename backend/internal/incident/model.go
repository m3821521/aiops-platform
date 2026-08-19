package incident

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// JSONMap 是 MySQL JSON 字段的通用 map 类型。
type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

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

// Incident 对应 incidents 表，是 AIOps 事件的统一聚合实体。
type Incident struct {
	ID           int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Title        string         `gorm:"size:512;not null" json:"title"`
	Severity     string         `gorm:"size:32;index;default:warning" json:"severity"`
	Status       string         `gorm:"size:32;index;default:open" json:"status"`
	Cluster      string         `gorm:"size:128;index" json:"cluster,omitempty"`
	Namespace    string         `gorm:"size:255;index" json:"namespace,omitempty"`
	Service      string         `gorm:"size:255;index" json:"service,omitempty"`
	ResourceType string         `gorm:"size:64" json:"resource_type,omitempty"`
	ResourceName string         `gorm:"size:255" json:"resource_name,omitempty"`
	RootCause    string         `gorm:"type:text" json:"root_cause,omitempty"`
	Confidence   float64        `json:"confidence,omitempty"` // 0.0~1.0
	Impact       string         `gorm:"type:text" json:"impact,omitempty"`
	Summary      string         `gorm:"type:text" json:"summary,omitempty"`
	SignalCount  int            `gorm:"default:0" json:"signal_count"`
	StartTime    time.Time      `gorm:"index;not null" json:"start_time"`
	EndTime      *time.Time     `json:"end_time,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	// 关联信号（查询时 Preload）。
	Signals []IncidentSignal `gorm:"foreignKey:IncidentID" json:"signals,omitempty"`
}

func (Incident) TableName() string { return "incidents" }

// Duration 返回事件持续时长，未结束则返回到现在的时长。
func (i *Incident) Duration() time.Duration {
	end := time.Now()
	if i.EndTime != nil {
		end = *i.EndTime
	}
	return end.Sub(i.StartTime)
}

// IncidentSignal 对应 incident_signals 表，统一存储所有信号来源。
type IncidentSignal struct {
	ID           int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID   int64          `gorm:"index;not null" json:"incident_id"`
	SignalType   string         `gorm:"size:32;index;not null" json:"signal_type"` // alert/anomaly/log/k8s_event/metric
	SignalID     string         `gorm:"size:255;index" json:"signal_id"`           // 外部唯一标识
	Title        string         `gorm:"size:512" json:"title"`
	Severity     string         `gorm:"size:32;index" json:"severity"`
	Cluster      string         `gorm:"size:128" json:"cluster,omitempty"`
	Namespace    string         `gorm:"size:255;index" json:"namespace,omitempty"`
	Service      string         `gorm:"size:255;index" json:"service,omitempty"`
	ResourceType string         `gorm:"size:64" json:"resource_type,omitempty"`
	ResourceName string         `gorm:"size:255" json:"resource_name,omitempty"`
	Timestamp    time.Time      `gorm:"index;not null" json:"timestamp"`
	Resolved     bool           `gorm:"default:false" json:"resolved"`
	ResolvedAt   *time.Time     `json:"resolved_at,omitempty"`
	Metadata     JSONMap        `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func (IncidentSignal) TableName() string { return "incident_signals" }

// ToSignal 将数据库记录转换为统一 Signal 结构。
func (s *IncidentSignal) ToSignal() Signal {
	return Signal{
		SignalType:   SignalType(s.SignalType),
		SignalID:     s.SignalID,
		Title:        s.Title,
		Severity:     s.Severity,
		Cluster:      s.Cluster,
		Namespace:    s.Namespace,
		Service:      s.Service,
		ResourceType: ResourceType(s.ResourceType),
		ResourceName: s.ResourceName,
		Timestamp:    s.Timestamp,
		Resolved:     s.Resolved,
		Metadata:     s.Metadata,
	}
}
