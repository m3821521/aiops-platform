// Package secret 提供统一的敏感信息存储抽象。
//
// 设计目标：
//   - Credential Manager 不直接依赖 AES 实现
//   - 支持多种 Secret Provider（本地加密、Kubernetes Secret、Vault 等）
//   - 所有敏感信息（password/token/api_key/private_key/kubeconfig）必须通过此接口存储
//
// 当前 V1 实现：LocalEncryptedSecretProvider（AES-256-GCM）
// 未来扩展：KubernetesSecretProvider、VaultSecretProvider、ExternalSecretProvider
package secret

import "context"

// SecretProvider 是敏感信息存储的抽象接口。
// 所有 Credential 的敏感数据必须通过此接口加密存储和读取。
type SecretProvider interface {
	// Type 返回 Provider 类型标识。
	Type() string

	// Encrypt 加密明文，返回可安全存储的密文字符串。
	Encrypt(ctx context.Context, plaintext string) (string, error)

	// Decrypt 解密密文，返回原始明文。
	Decrypt(ctx context.Context, ciphertext string) (string, error)

	// EncryptJSON 将任意结构体序列化为 JSON 后加密。
	// 用于存储包含多个字段的 Credential（如 username+password）。
	EncryptJSON(ctx context.Context, data interface{}) (string, error)

	// DecryptJSON 解密密文并反序列化为目标结构体。
	DecryptJSON(ctx context.Context, ciphertext string, target interface{}) error

	// Validate 检查 Provider 配置是否有效（如密钥长度、连接状态等）。
	Validate(ctx context.Context) error
}

// CredentialType 定义支持的凭证类型。
type CredentialType string

const (
	// CredentialUsernamePassword 用户名密码类型。
	CredentialUsernamePassword CredentialType = "username_password"
	// CredentialToken Bearer Token 类型。
	CredentialToken CredentialType = "token"
	// CredentialAPIKey API Key 类型。
	CredentialAPIKey CredentialType = "api_key"
	// CredentialTLS TLS 证书类型（certificate + private_key）。
	CredentialTLS CredentialType = "tls"
	// CredentialKubeconfig Kubernetes kubeconfig 文件内容。
	CredentialKubeconfig CredentialType = "kubeconfig"
)

// UsernamePasswordData 用户名密码凭证的数据结构。
type UsernamePasswordData struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenData Token 凭证的数据结构。
type TokenData struct {
	Token string `json:"token"`
}

// APIKeyData API Key 凭证的数据结构。
type APIKeyData struct {
	APIKey string `json:"api_key"`
}

// TLSData TLS 证书凭证的数据结构。
type TLSData struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
	CA          string `json:"ca,omitempty"`
}

// KubeconfigData kubeconfig 凭证的数据结构。
type KubeconfigData struct {
	Kubeconfig string `json:"kubeconfig"`
}

// SensitiveFields 定义需要脱敏的字段名，用于日志和 API 响应过滤。
var SensitiveFields = []string{
	"password",
	"token",
	"api_key",
	"apikey",
	"secret",
	"private_key",
	"privatekey",
	"kubeconfig",
	"credential",
	"authorization",
	"auth",
}

// MaskValue 对敏感值进行脱敏处理。
// 例如：sk-abc123def456 → sk-***************
func MaskValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	// 保留前 4 个字符，其余用 * 替换
	prefix := value[:4]
	masked := make([]byte, len(value)-4)
	for i := range masked {
		masked[i] = '*'
	}
	return prefix + string(masked)
}
