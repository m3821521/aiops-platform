package servicehealth

import (
	"time"

	"gorm.io/gorm"
)

// WorkloadType 标识 Service 关联的 Kubernetes 工作负载类型。
type WorkloadType string

const (
	WorkloadTypeDeployment  WorkloadType = "deployment"
	WorkloadTypeStatefulSet WorkloadType = "statefulset"
	WorkloadTypeDaemonSet   WorkloadType = "daemonset"
	// WorkloadTypeUnknown 表示无法通过 selector + ownerReference 确定工作负载，
	// 或 Service 没有 selector（如 ExternalName / 特殊 Headless Service）。
	WorkloadTypeUnknown WorkloadType = "unknown"
)

// Service 是平台级 Service Model，区别于 Kubernetes Service Object。
// 它是 Service Health / Signals / Dependencies / Incidents / RCA 的统一锚点。
//
// Identity: cluster + namespace + name（复合唯一索引）。
//
// 本阶段不包含 healthStatus / healthScore / signalCount / incidentCount /
// lastHealthCheckAt 等动态计算字段，这些属于后续 Phase。
type Service struct {
	ID               int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Name             string         `gorm:"size:255;not null;uniqueIndex:idx_svc_identity,priority:3" json:"name"`
	Namespace        string         `gorm:"size:255;not null;uniqueIndex:idx_svc_identity,priority:2" json:"namespace"`
	Cluster          string         `gorm:"size:128;not null;uniqueIndex:idx_svc_identity,priority:1" json:"cluster"`
	WorkloadType     WorkloadType   `gorm:"size:32;index" json:"workload_type"`
	WorkloadName     string         `gorm:"size:255" json:"workload_name,omitempty"`
	WorkloadSelector string         `gorm:"type:text" json:"workload_selector,omitempty"` // JSON-encoded selector map
	ServiceType      string         `gorm:"size:32" json:"service_type,omitempty"`        // ClusterIP/NodePort/LoadBalancer/ExternalName/Headless
	Owner            string         `gorm:"size:255" json:"owner,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 返回数据库表名。
func (Service) TableName() string { return "services" }
