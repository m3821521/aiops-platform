package connection

import (
	"context"
	"errors"
	"time"
)

// ConnectionChangeCallback 是 Connection 变更回调函数类型。
// 在 Connection 创建/更新/删除/启用/禁用成功后调用，
// 用于动态更新 Provider（K8s cluster.Manager、Prometheus、ES 等）。
// connType 为变更的 Connection 类型，可能为空（删除时）。
type ConnectionChangeCallback func(ctx context.Context, connType ConnectionType)

// ConnectionService 是 Connection 的业务逻辑层。
//
// 安全原则：
//   - Connection 只保存非敏感信息
//   - 敏感信息通过 Credential + SecretProvider 管理
//   - API 响应不返回敏感信息
//   - 删除 Connection 时不级联删除 Credential
type ConnectionService struct {
	repo              *ConnectionRepository
	credentialService *CredentialService
	onChanged         []ConnectionChangeCallback
}

// NewConnectionService 创建 Connection Service。
func NewConnectionService(repo *ConnectionRepository, credentialService *CredentialService) *ConnectionService {
	return &ConnectionService{
		repo:              repo,
		credentialService: credentialService,
		onChanged:         nil,
	}
}

// RegisterOnChanged 注册 Connection 变更回调。
// 回调在 Create/Update/Delete/Enable/Disable 成功后异步执行（goroutine），
// 避免阻塞 API 响应。回调内部应自行处理错误和 panic。
func (s *ConnectionService) RegisterOnChanged(cb ConnectionChangeCallback) {
	s.onChanged = append(s.onChanged, cb)
}

// notifyChanged 触发所有变更回调（异步执行，不阻塞 API）。
func (s *ConnectionService) notifyChanged(ctx context.Context, connType ConnectionType) {
	if len(s.onChanged) == 0 {
		return
	}
	for _, cb := range s.onChanged {
		go func(callback ConnectionChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
					// 防止回调 panic 影响整个进程
				}
			}()
			callback(ctx, connType)
		}(cb)
	}
}

// Create 创建新的 Connection。
// 如果请求中包含内联 Credential，则自动创建 Credential 并关联。
func (s *ConnectionService) Create(ctx context.Context, req CreateConnectionRequest, userID int64) (*ConnectionView, error) {
	// 验证请求
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	conn := &Connection{
		Name:          req.Name,
		Type:          req.Type,
		Environment:   req.Environment,
		Endpoint:      req.Endpoint,
		Config:        req.Config,
		CredentialID:  req.CredentialID,
		Enabled:       enabled,
		Status:        StatusUnknown,
		Description:   req.Description,
		CreatedBy:     userID,
		UpdatedBy:     userID,
	}
	if err := conn.Validate(); err != nil {
		return nil, err
	}

	// 检查名称是否已存在
	exists, err := s.repo.ExistsByName(ctx, req.Name, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("connection 名称已存在")
	}

	// 如果提供了内联 Credential，则创建它
	if req.Credential != nil {
		credView, err := s.credentialService.Create(ctx, *req.Credential, userID)
		if err != nil {
			return nil, errors.New("创建 credential 失败: " + err.Error())
		}
		conn.CredentialID = &credView.ID
	}

	// 如果指定了 CredentialID，验证它是否存在
	if conn.CredentialID != nil {
		cred, err := s.credentialService.GetByID(ctx, *conn.CredentialID)
		if err != nil || cred == nil {
			return nil, errors.New("指定的 credential 不存在")
		}
	}

	// 保存到数据库
	if err := s.repo.Create(ctx, conn); err != nil {
		return nil, err
	}

	// 触发变更回调，动态更新 Provider
	s.notifyChanged(ctx, conn.Type)

	return s.toView(ctx, conn), nil
}

// GetByID 根据 ID 获取 Connection（脱敏视图）。
func (s *ConnectionService) GetByID(ctx context.Context, id int64) (*ConnectionView, error) {
	conn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection 不存在")
	}
	return s.toView(ctx, conn), nil
}

// GetRawByID 根据 ID 获取原始 Connection（包含 CredentialID，用于内部 Provider 创建）。
func (s *ConnectionService) GetRawByID(ctx context.Context, id int64) (*Connection, error) {
	conn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection 不存在")
	}
	return conn, nil
}

// List 分页查询 Connection 列表（脱敏视图）。
func (s *ConnectionService) List(ctx context.Context, filter ConnectionFilter, page, pageSize int) ([]ConnectionView, int64, error) {
	connections, total, err := s.repo.List(ctx, filter, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	views := make([]ConnectionView, len(connections))
	for i, conn := range connections {
		views[i] = *s.toView(ctx, &conn)
	}
	return views, total, nil
}

// ListByType 根据类型获取所有启用的 Connection。
func (s *ConnectionService) ListByType(ctx context.Context, connType ConnectionType) ([]Connection, error) {
	return s.repo.ListByType(ctx, connType)
}

// Update 更新 Connection。
func (s *ConnectionService) Update(ctx context.Context, id int64, req UpdateConnectionRequest, userID int64) (*ConnectionView, error) {
	conn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection 不存在")
	}

	// 不允许修改 system_default connection
	if conn.IsSystemDefault {
		return nil, errors.New("system default connection 不允许修改")
	}

	// 更新字段
	if req.Name != nil {
		exists, err := s.repo.ExistsByName(ctx, *req.Name, &id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("connection 名称已存在")
		}
		conn.Name = *req.Name
	}
	if req.Environment != nil {
		conn.Environment = *req.Environment
	}
	if req.Endpoint != nil {
		conn.Endpoint = *req.Endpoint
	}
	if req.Config != nil {
		conn.Config = *req.Config
	}
	if req.CredentialID != nil {
		// 如果指定了新的 CredentialID，验证它是否存在
		cred, err := s.credentialService.GetByID(ctx, *req.CredentialID)
		if err != nil || cred == nil {
			return nil, errors.New("指定的 credential 不存在")
		}
		conn.CredentialID = req.CredentialID
	}
	if req.Enabled != nil {
		conn.Enabled = *req.Enabled
	}
	if req.Description != nil {
		conn.Description = *req.Description
	}

	conn.UpdatedBy = userID
	conn.UpdatedAt = time.Now()

	// 验证更新后的数据
	if err := conn.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, conn); err != nil {
		return nil, err
	}

	// 触发变更回调，动态更新 Provider
	s.notifyChanged(ctx, conn.Type)

	return s.toView(ctx, conn), nil
}

// Delete 删除 Connection。
// 注意：不级联删除关联的 Credential。
func (s *ConnectionService) Delete(ctx context.Context, id int64) error {
	conn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if conn == nil {
		return errors.New("connection 不存在")
	}
	if conn.IsSystemDefault {
		return errors.New("system default connection 不允许删除")
	}
	connType := conn.Type
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// 触发变更回调，动态更新 Provider
	s.notifyChanged(ctx, connType)
	return nil
}

// Enable 启用 Connection。
func (s *ConnectionService) Enable(ctx context.Context, id int64, userID int64) (*ConnectionView, error) {
	enabled := true
	return s.Update(ctx, id, UpdateConnectionRequest{Enabled: &enabled}, userID)
}

// Disable 禁用 Connection。
func (s *ConnectionService) Disable(ctx context.Context, id int64, userID int64) (*ConnectionView, error) {
	enabled := false
	return s.Update(ctx, id, UpdateConnectionRequest{Enabled: &enabled}, userID)
}

// UpdateStatus 更新 Connection 状态（由 Connection Test 调用）。
func (s *ConnectionService) UpdateStatus(ctx context.Context, id int64, status ConnectionStatus, lastError string) error {
	return s.repo.UpdateStatus(ctx, id, status, lastError)
}

// toView 将 Connection 转换为脱敏视图。
func (s *ConnectionService) toView(ctx context.Context, conn *Connection) *ConnectionView {
	view := &ConnectionView{
		ID:              conn.ID,
		Name:            conn.Name,
		Type:            conn.Type,
		Environment:     conn.Environment,
		Endpoint:        conn.Endpoint,
		Config:          conn.Config,
		CredentialID:    conn.CredentialID,
		Enabled:         conn.Enabled,
		Status:          conn.Status,
		LastCheckAt:     conn.LastCheckAt,
		LastError:       conn.LastError,
		Description:     conn.Description,
		IsSystemDefault: conn.IsSystemDefault,
		CreatedBy:       conn.CreatedBy,
		UpdatedBy:       conn.UpdatedBy,
		CreatedAt:       conn.CreatedAt,
		UpdatedAt:       conn.UpdatedAt,
	}

	// 如果有关联的 Credential，获取其类型（不返回内容）
	if conn.CredentialID != nil {
		cred, err := s.credentialService.GetByID(ctx, *conn.CredentialID)
		if err == nil && cred != nil {
			view.CredentialType = string(cred.Type)
		}
	}

	return view
}
