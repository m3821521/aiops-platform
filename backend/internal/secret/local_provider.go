package secret

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

// LocalEncryptedSecretProvider 是基于本地 AES-256-GCM 加密的 SecretProvider 实现。
//
// 安全说明：
//   - Master Key 必须通过环境变量或外部 Secret 管理注入，禁止硬编码到代码或 config.yaml
//   - 所有敏感数据在存储前都会被 AES-256-GCM 加密
//   - 加密使用随机 nonce，相同明文每次加密结果不同
//   - 支持 AAD（Additional Authenticated Data），当前使用空 AAD
//
// 未来扩展：
//   - KubernetesSecretProvider：使用 Kubernetes Secret 存储
//   - VaultSecretProvider：使用 HashiCorp Vault
//   - ExternalSecretProvider：使用外部 Secret 管理服务
type LocalEncryptedSecretProvider struct {
	key []byte // AES-256 密钥（32 字节）
}

// NewLocalEncryptedSecretProvider 创建本地加密 SecretProvider。
//
// 参数：
//   - masterKey: AES-256 主密钥，必须至少 32 字节。不足 32 字节会自动填充（仅用于开发环境）。
//
// 安全建议：
//   - 生产环境必须通过环境变量 SECRET_MASTER_KEY 注入 32 字节随机密钥
//   - 禁止将 masterKey 提交到 Git 或写入 config.yaml
func NewLocalEncryptedSecretProvider(masterKey string) (*LocalEncryptedSecretProvider, error) {
	if masterKey == "" {
		return nil, errors.New("master key 不能为空")
	}

	key := []byte(masterKey)
	if len(key) < 32 {
		// 不足 32 字节则填充（仅用于开发环境，生产环境应使用完整 32 字节密钥）
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	}

	// 验证密钥可以创建 AES cipher
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}
	_, err = cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &LocalEncryptedSecretProvider{key: key[:32]}, nil
}

// Type 返回 Provider 类型标识。
func (p *LocalEncryptedSecretProvider) Type() string {
	return "local_encrypted_aes256_gcm"
}

// Encrypt 使用 AES-256-GCM 加密明文，返回 Base64 编码的密文（包含 nonce）。
func (p *LocalEncryptedSecretProvider) Encrypt(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(p.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Seal 将 nonce 前置到密文前，格式：nonce + ciphertext + tag
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 使用 AES-256-GCM 解密密文。
func (p *LocalEncryptedSecretProvider) Decrypt(ctx context.Context, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", errors.New("密文 Base64 解码失败")
	}

	block, err := aes.NewCipher(p.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("密文过短，缺少 nonce")
	}

	nonce, encrypted := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", errors.New("解密失败，密钥可能不正确或数据已损坏")
	}

	return string(plaintext), nil
}

// EncryptJSON 将任意结构体序列化为 JSON 后加密。
func (p *LocalEncryptedSecretProvider) EncryptJSON(ctx context.Context, data interface{}) (string, error) {
	if data == nil {
		return "", nil
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return p.Encrypt(ctx, string(jsonBytes))
}

// DecryptJSON 解密密文并反序列化为目标结构体。
func (p *LocalEncryptedSecretProvider) DecryptJSON(ctx context.Context, ciphertext string, target interface{}) error {
	if ciphertext == "" {
		return nil
	}

	plaintext, err := p.Decrypt(ctx, ciphertext)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(plaintext), target)
}

// Validate 检查 Provider 配置是否有效。
func (p *LocalEncryptedSecretProvider) Validate(ctx context.Context) error {
	if len(p.key) != 32 {
		return errors.New("AES 密钥长度必须为 32 字节")
	}

	// 执行一次加密解密往返测试
	testPlaintext := "validate-secret-provider"
	ciphertext, err := p.Encrypt(ctx, testPlaintext)
	if err != nil {
		return err
	}

	decrypted, err := p.Decrypt(ctx, ciphertext)
	if err != nil {
		return err
	}

	if decrypted != testPlaintext {
		return errors.New("加密解密往返测试失败")
	}

	return nil
}
