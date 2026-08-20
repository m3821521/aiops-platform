// Package connection 提供统一的外部系统连接管理。
//
// 设计目标：
//   - 一套代码 + 一个 Docker Image + 不同 Connection 即可运行 DEV/TEST/STAGING/PROD
//   - Connection Metadata 与 Credential 严格分离
//   - 支持多集群、多 Prometheus、多 Elasticsearch 等
//   - 所有敏感信息通过 SecretProvider 加密存储
//
// 支持的 Connection 类型：
//   - kubernetes
//   - prometheus
//   - grafana
//   - elasticsearch
//   - mysql
//   - redis
//   - jenkins
//   - argocd
package connection

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// ConnectionType 定义支持的外部系统连接类型。
type ConnectionType string

const (
	// TypeKubernetes Kubernetes 集群连接。
	TypeKubernetes ConnectionType = "kubernetes"
	// TypePrometheus Prometheus 监控连接。
	TypePrometheus ConnectionType = "prometheus"
	// TypeGrafana Grafana 可视化连接。
	TypeGrafana ConnectionType = "grafana"
	// TypeElasticsearch Elasticsearch/OpenSearch 日志连接。
	TypeElasticsearch ConnectionType = "elasticsearch"
	// TypeMySQL MySQL 数据库连接。
	TypeMySQL ConnectionType = "mysql"
	// TypeRedis Redis 缓存连接。
	TypeRedis ConnectionType = "redis"
	// TypeJenkins Jenkins CI 连接。
	TypeJenkins ConnectionType = "jenkins"
	// TypeArgoCD ArgoCD CD 连接。
	TypeArgoCD ConnectionType = "argocd"
)

// ConnectionStatus 定义连接状态。
type ConnectionStatus string

const (
	// StatusUnknown 未知状态（未测试）。
	StatusUnknown ConnectionStatus = "unknown"
	// StatusAvailable 可用（最近测试成功）。
	StatusAvailable ConnectionStatus = "available"
	// StatusUnavailable 不可用（最近测试失败）。
	StatusUnavailable ConnectionStatus = "unavailable"
)

// Environment 定义部署环境。
type Environment string

const (
	// EnvDev 开发环境。
	EnvDev Environment = "dev"
	// EnvTest 测试环境。
	EnvTest Environment = "test"
	// EnvStaging 预发布环境。
	EnvStaging Environment = "staging"
	// EnvProd 生产环境。
	EnvProd Environment = "prod"
)

// ConfigMap 是连接的非敏感配置，使用 JSON 存储。
// 例如：timeout、index、tls_skip_verify 等。
type ConfigMap map[string]interface{}

// Value 实现 driver.Valuer 接口，用于 GORM 存储。
func (c ConfigMap) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan 实现 sql.Scanner 接口，用于 GORM 读取。
func (c *ConfigMap) Scan(value interface{}) error {
	if value == nil {
		*c = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("ConfigMap.Scan: 不支持的类型")
	}

	return json.Unmarshal(bytes, c)
}

// Connection 是外部系统连接的元数据模型。
//
// 重要：
//   - Connection 只保存非敏感信息（endpoint、type、environment、config）
//   - 敏感信息（password、token、api_key、private_key、kubeconfig）必须保存在 Credential 中
//   - API 响应绝对不能返回敏感信息
//
// 数据库表：external_connections
type Connection struct {
	ID            int64            `gorm:"primaryKey" json:"id"`
	Name          string           `gorm:"size:100;not null;index" json:"name"`
	Type          ConnectionType   `gorm:"size:50;not null;index" json:"type"`
	Environment   Environment      `gorm:"size:50;not null;index" json:"environment"`
	Endpoint      string           `gorm:"size:500;not null" json:"endpoint"`
	Config        ConfigMap        `gorm:"type:json" json:"config,omitempty"`
	CredentialID  *int64           `gorm:"index" json:"credential_id,omitempty"`
	Enabled       bool             `gorm:"default:true;index" json:"enabled"`
	Status        ConnectionStatus `gorm:"size:20;default:unknown;index" json:"status"`
	LastCheckAt   *time.Time       `json:"last_check_at,omitempty"`
	LastError     string           `gorm:"size:1000" json:"last_error,omitempty"`
	Description   string           `gorm:"size:500" json:"description,omitempty"`
	IsSystemDefault bool           `gorm:"default:false" json:"is_system_default"`
	TenantID      *int64           `gorm:"index" json:"tenant_id,omitempty"` // 预留多租户字段
	CreatedBy     int64            `gorm:"default:0" json:"created_by"`
	UpdatedBy     int64            `gorm:"default:0" json:"updated_by"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// TableName 指定数据库表名。
func (Connection) TableName() string {
	return "external_connections"
}

// Validate 验证 Connection 数据有效性。
func (c *Connection) Validate() error {
	if c.Name == "" {
		return errors.New("connection name 不能为空")
	}
	if c.Type == "" {
		return errors.New("connection type 不能为空")
	}
	if c.Endpoint == "" {
		return errors.New("connection endpoint 不能为空")
	}

	// 验证 type 是否为支持的类型
	switch c.Type {
	case TypeKubernetes, TypePrometheus, TypeGrafana, TypeElasticsearch,
		TypeMySQL, TypeRedis, TypeJenkins, TypeArgoCD:
		// 有效类型
	default:
		return errors.New("不支持的 connection type: " + string(c.Type))
	}

	return nil
}

// ConnectionView 是 API 响应使用的 Connection 视图，确保不包含敏感信息。
type ConnectionView struct {
	ID              int64            `json:"id"`
	Name            string           `json:"name"`
	Type            ConnectionType   `json:"type"`
	Environment     Environment      `json:"environment"`
	Endpoint        string           `json:"endpoint"`
	Config          ConfigMap        `json:"config,omitempty"`
	CredentialID    *int64           `json:"credential_id,omitempty"`
	CredentialType  string           `json:"credential_type,omitempty"` // 只返回类型，不返回内容
	Enabled         bool             `json:"enabled"`
	Status          ConnectionStatus `json:"status"`
	LastCheckAt     *time.Time       `json:"last_check_at,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	Description     string           `json:"description,omitempty"`
	IsSystemDefault bool             `json:"is_system_default"`
	CreatedBy       int64            `json:"created_by"`
	UpdatedBy       int64            `json:"updated_by"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}
