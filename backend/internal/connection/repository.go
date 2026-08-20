package connection

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ConnectionRepository 是 Connection 的数据访问层。
type ConnectionRepository struct {
	db *gorm.DB
}

// NewConnectionRepository 创建 Connection Repository。
func NewConnectionRepository(db *gorm.DB) *ConnectionRepository {
	return &ConnectionRepository{db: db}
}

// ConnectionFilter 是 Connection 查询过滤条件。
type ConnectionFilter struct {
	Type        ConnectionType
	Environment Environment
	Enabled     *bool
	Status      ConnectionStatus
	Name        string
}

// Create 创建新的 Connection。
func (r *ConnectionRepository) Create(ctx context.Context, conn *Connection) error {
	if conn == nil {
		return errors.New("connection 不能为空")
	}
	return r.db.WithContext(ctx).Create(conn).Error
}

// GetByID 根据 ID 获取 Connection。
func (r *ConnectionRepository) GetByID(ctx context.Context, id int64) (*Connection, error) {
	var conn Connection
	err := r.db.WithContext(ctx).First(&conn, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &conn, nil
}

// GetByName 根据名称获取 Connection。
func (r *ConnectionRepository) GetByName(ctx context.Context, name string) (*Connection, error) {
	var conn Connection
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&conn).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &conn, nil
}

// List 分页查询 Connection 列表。
func (r *ConnectionRepository) List(ctx context.Context, filter ConnectionFilter, page, pageSize int) ([]Connection, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&Connection{})

	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.Environment != "" {
		query = query.Where("environment = ?", filter.Environment)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Name != "" {
		query = query.Where("name LIKE ?", "%"+filter.Name+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var connections []Connection
	err := query.Session(&gorm.Session{}).Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&connections).Error
	if err != nil {
		return nil, 0, err
	}

	return connections, total, nil
}

// ListByType 根据类型获取所有启用的 Connection。
func (r *ConnectionRepository) ListByType(ctx context.Context, connType ConnectionType) ([]Connection, error) {
	var connections []Connection
	err := r.db.WithContext(ctx).
		Where("type = ? AND enabled = ?", connType, true).
		Order("name ASC").
		Find(&connections).Error
	return connections, err
}

// Update 更新 Connection。
func (r *ConnectionRepository) Update(ctx context.Context, conn *Connection) error {
	if conn == nil || conn.ID == 0 {
		return errors.New("connection ID 不能为空")
	}
	return r.db.WithContext(ctx).Save(conn).Error
}

// UpdateStatus 更新 Connection 状态和测试信息。
func (r *ConnectionRepository) UpdateStatus(ctx context.Context, id int64, status ConnectionStatus, lastError string) error {
	updates := map[string]interface{}{
		"status":         status,
		"last_check_at":  time.Now(),
		"last_error":     lastError,
	}
	return r.db.WithContext(ctx).Model(&Connection{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除 Connection。
func (r *ConnectionRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Connection{}, id).Error
}

// ExistsByName 检查名称是否已存在。
func (r *ConnectionRepository) ExistsByName(ctx context.Context, name string, excludeID *int64) (bool, error) {
	query := r.db.WithContext(ctx).Model(&Connection{}).Where("name = ?", name)
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

// CredentialRepository 是 Credential 的数据访问层。
type CredentialRepository struct {
	db *gorm.DB
}

// NewCredentialRepository 创建 Credential Repository。
func NewCredentialRepository(db *gorm.DB) *CredentialRepository {
	return &CredentialRepository{db: db}
}

// Create 创建新的 Credential。
func (r *CredentialRepository) Create(ctx context.Context, cred *Credential) error {
	if cred == nil {
		return errors.New("credential 不能为空")
	}
	return r.db.WithContext(ctx).Create(cred).Error
}

// GetByID 根据 ID 获取 Credential。
func (r *CredentialRepository) GetByID(ctx context.Context, id int64) (*Credential, error) {
	var cred Credential
	err := r.db.WithContext(ctx).First(&cred, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cred, nil
}

// List 分页查询 Credential 列表。
func (r *CredentialRepository) List(ctx context.Context, page, pageSize int) ([]Credential, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := r.db.WithContext(ctx).Model(&Credential{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var credentials []Credential
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&credentials).Error
	if err != nil {
		return nil, 0, err
	}

	return credentials, total, nil
}

// Update 更新 Credential。
func (r *CredentialRepository) Update(ctx context.Context, cred *Credential) error {
	if cred == nil || cred.ID == 0 {
		return errors.New("credential ID 不能为空")
	}
	return r.db.WithContext(ctx).Save(cred).Error
}

// Delete 删除 Credential。
func (r *CredentialRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Credential{}, id).Error
}

// ExistsByName 检查名称是否已存在。
func (r *CredentialRepository) ExistsByName(ctx context.Context, name string, excludeID *int64) (bool, error) {
	query := r.db.WithContext(ctx).Model(&Credential{}).Where("name = ?", name)
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

// IsInUse 检查 Credential 是否被 Connection 引用。
func (r *CredentialRepository) IsInUse(ctx context.Context, credentialID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Connection{}).Where("credential_id = ?", credentialID).Count(&count).Error
	return count > 0, err
}
