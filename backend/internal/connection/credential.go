package connection

import (
	"errors"
	"time"

	"github.com/aiops/aiops-platform/internal/secret"
)

// Credential 是敏感凭证的元数据模型。
//
// 重要：
//   - Credential 只保存元数据（name、type、关联信息）
//   - 实际敏感数据（encrypted_data）通过 SecretProvider 加密存储
//   - API 响应绝对不能返回 encrypted_data 或解密后的明文
//   - 日志中绝对不能出现敏感信息
//
// 数据库表：credentials
type Credential struct {
	ID            int64                 `gorm:"primaryKey" json:"id"`
	Name          string                `gorm:"size:100;not null;index" json:"name"`
	Type          secret.CredentialType `gorm:"size:50;not null;index" json:"type"`
	EncryptedData string                `gorm:"type:text" json:"-"` // 加密后的敏感数据，不返回给前端
	Description   string                `gorm:"size:500" json:"description,omitempty"`
	TenantID      *int64                `gorm:"index" json:"tenant_id,omitempty"` // 预留多租户字段
	CreatedBy     int64                 `gorm:"default:0" json:"created_by"`
	UpdatedBy     int64                 `gorm:"default:0" json:"updated_by"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// TableName 指定数据库表名。
func (Credential) TableName() string {
	return "credentials"
}

// Validate 验证 Credential 数据有效性。
func (c *Credential) Validate() error {
	if c.Name == "" {
		return errors.New("credential name 不能为空")
	}
	if c.Type == "" {
		return errors.New("credential type 不能为空")
	}

	// 验证 type 是否为支持的类型
	switch c.Type {
	case secret.CredentialUsernamePassword, secret.CredentialToken,
		secret.CredentialAPIKey, secret.CredentialTLS, secret.CredentialKubeconfig:
		// 有效类型
	default:
		return errors.New("不支持的 credential type: " + string(c.Type))
	}

	return nil
}

// CredentialView 是 API 响应使用的 Credential 视图，确保不包含敏感信息。
type CredentialView struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	Type        secret.CredentialType `json:"type"`
	Description string                `json:"description,omitempty"`
	// MaskedData 是脱敏后的敏感数据预览，例如：{"username": "admin", "password": "****"}
	MaskedData map[string]string `json:"masked_data,omitempty"`
	CreatedBy  int64             `json:"created_by"`
	UpdatedBy  int64             `json:"updated_by"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// BuildMaskedData 根据 Credential 类型构建脱敏数据预览。
// 此函数只返回字段名和脱敏后的值，不返回明文。
func BuildMaskedData(credType secret.CredentialType, decryptedData map[string]string) map[string]string {
	masked := make(map[string]string)

	for key, value := range decryptedData {
		// 检查是否为敏感字段
		isSensitive := false
		for _, sensitive := range secret.SensitiveFields {
			if key == sensitive {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			masked[key] = secret.MaskValue(value)
		} else {
			masked[key] = value
		}
	}

	return masked
}

// CreateCredentialRequest 创建 Credential 的请求结构。
type CreateCredentialRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        secret.CredentialType `json:"type" binding:"required"`
	Description string                 `json:"description"`
	// Data 是明文敏感数据，会被加密后存储。
	// 根据 type 不同，需要的字段不同：
	//   - username_password: username, password
	//   - token: token
	//   - api_key: api_key
	//   - tls: certificate, private_key, ca
	//   - kubeconfig: kubeconfig
	Data map[string]string `json:"data" binding:"required"`
}

// UpdateCredentialRequest 更新 Credential 的请求结构。
type UpdateCredentialRequest struct {
	Name        *string                `json:"name"`
	Description *string                `json:"description"`
	// Data 是新的明文敏感数据，如果提供则会重新加密存储。
	Data map[string]string `json:"data"`
}

// CreateConnectionRequest 创建 Connection 的请求结构。
type CreateConnectionRequest struct {
	Name        string           `json:"name" binding:"required"`
	Type        ConnectionType   `json:"type" binding:"required"`
	Environment Environment      `json:"environment" binding:"required"`
	Endpoint    string           `json:"endpoint" binding:"required"`
	Config      ConfigMap        `json:"config"`
	CredentialID *int64          `json:"credential_id"`
	Enabled     *bool            `json:"enabled"`
	Description string           `json:"description"`
	// Credential 可以内联创建，如果提供则会自动创建 Credential 并关联。
	Credential *CreateCredentialRequest `json:"credential,omitempty"`
}

// UpdateConnectionRequest 更新 Connection 的请求结构。
type UpdateConnectionRequest struct {
	Name         *string         `json:"name"`
	Environment  *Environment    `json:"environment"`
	Endpoint     *string         `json:"endpoint"`
	Config       *ConfigMap      `json:"config"`
	CredentialID *int64          `json:"credential_id"`
	Enabled      *bool           `json:"enabled"`
	Description  *string         `json:"description"`
}

// TestConnectionResult 连接测试结果。
type TestConnectionResult struct {
	Status       ConnectionStatus `json:"status"`
	LatencyMs    int64            `json:"latency_ms"`
	ErrorCode    string           `json:"error_code,omitempty"`
	ErrorMessage string           `json:"error_message,omitempty"`
	CheckedAt    time.Time        `json:"checked_at"`
}
