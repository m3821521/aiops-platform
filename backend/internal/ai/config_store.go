package ai

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"gorm.io/gorm"
)

// AIConfig 是 AI 配置的数据库存储模型。
// API Key 使用 AES 加密存储，不保存明文。
type AIConfig struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Provider     string    `gorm:"size:50;default:openai" json:"provider"`
	BaseURL      string    `gorm:"size:500" json:"base_url"`
	APIKeyEnc    string    `gorm:"column:api_key_enc;size:1000" json:"-"` // 加密后的 API Key，不返回给前端
	Model        string    `gorm:"size:100" json:"model"`
	Enabled      bool      `gorm:"default:true" json:"enabled"`
	UpdatedBy    int64     `gorm:"default:0" json:"updated_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AIConfig) TableName() string {
	return "ai_configs"
}

// AIConfigRepository 是 AI 配置的数据库操作。
type AIConfigRepository struct {
	db     *gorm.DB
	secret []byte // AES 加密密钥
}

// NewAIConfigRepository 创建 AI 配置 Repository。
// secret 是 AES-256 加密密钥（32 字节）。
func NewAIConfigRepository(db *gorm.DB, secret string) *AIConfigRepository {
	key := []byte(secret)
	if len(key) < 32 {
		// 不足 32 字节则填充
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	}
	return &AIConfigRepository{db: db, secret: key[:32]}
}

// encrypt 使用 AES-GCM 加密明文。
func (r *AIConfigRepository) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(r.secret)
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
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt 使用 AES-GCM 解密密文。
func (r *AIConfigRepository) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(r.secret)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// Get 获取当前 AI 配置（单例模式，只有一条记录）。
func (r *AIConfigRepository) Get(ctx context.Context) (*AIConfig, error) {
	var cfg AIConfig
	err := r.db.WithContext(ctx).Order("id DESC").First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// GetAPIKey 获取解密后的 API Key。
func (r *AIConfigRepository) GetAPIKey(ctx context.Context) (string, error) {
	cfg, err := r.Get(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || cfg.APIKeyEnc == "" {
		return "", nil
	}
	return r.decrypt(cfg.APIKeyEnc)
}

// Save 保存 AI 配置（UPSERT）。
// apiKey 是明文 API Key，会被加密后存储。
func (r *AIConfigRepository) Save(ctx context.Context, provider, baseURL, apiKey, model string, enabled bool, userID int64) (*AIConfig, error) {
	cfg, err := r.Get(ctx)
	if err != nil {
		return nil, err
	}

	var encKey string
	if apiKey != "" {
		encKey, err = r.encrypt(apiKey)
		if err != nil {
			return nil, err
		}
	}

	if cfg == nil {
		// 新建
		cfg = &AIConfig{
			Provider:  provider,
			BaseURL:   baseURL,
			APIKeyEnc: encKey,
			Model:     model,
			Enabled:   enabled,
			UpdatedBy: userID,
		}
		if err := r.db.WithContext(ctx).Create(cfg).Error; err != nil {
			return nil, err
		}
	} else {
		// 更新
		updates := map[string]interface{}{
			"provider":   provider,
			"base_url":   baseURL,
			"model":      model,
			"enabled":    enabled,
			"updated_by": userID,
		}
		if apiKey != "" {
			updates["api_key_enc"] = encKey
		}
		if err := r.db.WithContext(ctx).Model(cfg).Updates(updates).Error; err != nil {
			return nil, err
		}
		// 重新读取
		cfg, err = r.Get(ctx)
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// IsConfigured 检查是否已配置 API Key。
func (r *AIConfigRepository) IsConfigured(ctx context.Context) (bool, error) {
	key, err := r.GetAPIKey(ctx)
	if err != nil {
		return false, err
	}
	return key != "", nil
}
