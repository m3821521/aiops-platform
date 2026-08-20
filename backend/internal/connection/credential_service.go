package connection

import (
	"context"
	"errors"
	"time"

	"github.com/aiops/aiops-platform/internal/secret"
)

// CredentialService 是 Credential 的业务逻辑层。
//
// 安全原则：
//   - 所有敏感数据通过 SecretProvider 加密存储
//   - API 响应只返回脱敏后的数据预览
//   - 日志中绝对不能出现敏感信息
//   - 删除 Credential 前检查是否被 Connection 引用
type CredentialService struct {
	repo       *CredentialRepository
	secretProv secret.SecretProvider
}

// NewCredentialService 创建 Credential Service。
func NewCredentialService(repo *CredentialRepository, secretProv secret.SecretProvider) *CredentialService {
	return &CredentialService{
		repo:       repo,
		secretProv: secretProv,
	}
}

// Create 创建新的 Credential。
// data 是明文敏感数据，会被加密后存储。
func (s *CredentialService) Create(ctx context.Context, req CreateCredentialRequest, userID int64) (*CredentialView, error) {
	// 验证请求
	cred := &Credential{
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		CreatedBy:   userID,
		UpdatedBy:   userID,
	}
	if err := cred.Validate(); err != nil {
		return nil, err
	}

	// 验证数据字段
	if err := validateCredentialData(req.Type, req.Data); err != nil {
		return nil, err
	}

	// 检查名称是否已存在
	exists, err := s.repo.ExistsByName(ctx, req.Name, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("credential 名称已存在")
	}

	// 加密敏感数据
	encrypted, err := s.secretProv.EncryptJSON(ctx, req.Data)
	if err != nil {
		return nil, errors.New("加密敏感数据失败")
	}
	cred.EncryptedData = encrypted

	// 保存到数据库
	if err := s.repo.Create(ctx, cred); err != nil {
		return nil, err
	}

	return s.toView(ctx, cred), nil
}

// GetByID 根据 ID 获取 Credential（脱敏视图）。
func (s *CredentialService) GetByID(ctx context.Context, id int64) (*CredentialView, error) {
	cred, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, errors.New("credential 不存在")
	}
	return s.toView(ctx, cred), nil
}

// GetDecryptedData 获取解密后的敏感数据（仅供内部使用，不通过 API 暴露）。
func (s *CredentialService) GetDecryptedData(ctx context.Context, id int64) (map[string]string, error) {
	cred, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, errors.New("credential 不存在")
	}

	var data map[string]string
	if err := s.secretProv.DecryptJSON(ctx, cred.EncryptedData, &data); err != nil {
		return nil, errors.New("解密敏感数据失败")
	}
	return data, nil
}

// List 分页查询 Credential 列表（脱敏视图）。
func (s *CredentialService) List(ctx context.Context, page, pageSize int) ([]CredentialView, int64, error) {
	credentials, total, err := s.repo.List(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	views := make([]CredentialView, len(credentials))
	for i, cred := range credentials {
		views[i] = *s.toView(ctx, &cred)
	}
	return views, total, nil
}

// Update 更新 Credential。
func (s *CredentialService) Update(ctx context.Context, id int64, req UpdateCredentialRequest, userID int64) (*CredentialView, error) {
	cred, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, errors.New("credential 不存在")
	}

	// 更新名称
	if req.Name != nil {
		exists, err := s.repo.ExistsByName(ctx, *req.Name, &id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("credential 名称已存在")
		}
		cred.Name = *req.Name
	}

	// 更新描述
	if req.Description != nil {
		cred.Description = *req.Description
	}

	// 更新敏感数据（如果提供）
	if req.Data != nil {
		if err := validateCredentialData(cred.Type, req.Data); err != nil {
			return nil, err
		}
		encrypted, err := s.secretProv.EncryptJSON(ctx, req.Data)
		if err != nil {
			return nil, errors.New("加密敏感数据失败")
		}
		cred.EncryptedData = encrypted
	}

	cred.UpdatedBy = userID
	cred.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, cred); err != nil {
		return nil, err
	}

	return s.toView(ctx, cred), nil
}

// Delete 删除 Credential。
// 删除前检查是否被 Connection 引用，如果被引用则拒绝删除。
func (s *CredentialService) Delete(ctx context.Context, id int64) error {
	inUse, err := s.repo.IsInUse(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return errors.New("credential 正在被 Connection 引用，无法删除")
	}
	return s.repo.Delete(ctx, id)
}

// toView 将 Credential 转换为脱敏视图。
func (s *CredentialService) toView(ctx context.Context, cred *Credential) *CredentialView {
	view := &CredentialView{
		ID:          cred.ID,
		Name:        cred.Name,
		Type:        cred.Type,
		Description: cred.Description,
		CreatedBy:   cred.CreatedBy,
		UpdatedBy:   cred.UpdatedBy,
		CreatedAt:   cred.CreatedAt,
		UpdatedAt:   cred.UpdatedAt,
	}

	// 解密并脱敏数据（只返回脱敏预览，不返回明文）
	if cred.EncryptedData != "" {
		var data map[string]string
		if err := s.secretProv.DecryptJSON(ctx, cred.EncryptedData, &data); err == nil {
			view.MaskedData = BuildMaskedData(cred.Type, data)
		}
	}

	return view
}

// validateCredentialData 根据 Credential 类型验证必需字段。
func validateCredentialData(credType secret.CredentialType, data map[string]string) error {
	if data == nil || len(data) == 0 {
		return errors.New("credential data 不能为空")
	}

	switch credType {
	case secret.CredentialUsernamePassword:
		if data["username"] == "" {
			return errors.New("username_password 类型需要 username 字段")
		}
		if data["password"] == "" {
			return errors.New("username_password 类型需要 password 字段")
		}
	case secret.CredentialToken:
		if data["token"] == "" {
			return errors.New("token 类型需要 token 字段")
		}
	case secret.CredentialAPIKey:
		if data["api_key"] == "" {
			return errors.New("api_key 类型需要 api_key 字段")
		}
	case secret.CredentialTLS:
		if data["certificate"] == "" {
			return errors.New("tls 类型需要 certificate 字段")
		}
		if data["private_key"] == "" {
			return errors.New("tls 类型需要 private_key 字段")
		}
	case secret.CredentialKubeconfig:
		if data["kubeconfig"] == "" {
			return errors.New("kubeconfig 类型需要 kubeconfig 字段")
		}
	default:
		return errors.New("不支持的 credential type: " + string(credType))
	}

	return nil
}
