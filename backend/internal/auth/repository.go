package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository 用户数据访问层。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建用户 Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// FindByUsername 根据用户名查找用户（含角色和权限）。
func (r *Repository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		Where("username = ?", username).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// FindByID 根据 ID 查找用户（含角色和权限）。
func (r *Repository) FindByID(ctx context.Context, id int64) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// UpdateLastLogin 更新用户最后登录时间。
func (r *Repository) UpdateLastLogin(ctx context.Context, userID int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&User{}).
		Where("id = ?", userID).
		Update("last_login_at", now).Error
}

// List 列出用户（分页）。
func (r *Repository) List(ctx context.Context, page, pageSize int) ([]User, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var users []User
	var total int64

	r.db.WithContext(ctx).Model(&User{}).Count(&total)
	err := r.db.WithContext(ctx).
		Preload("Roles").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&users).Error
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// Create 创建用户。
func (r *Repository) Create(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// Update 更新用户。
func (r *Repository) Update(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// Delete 删除用户（软删除）。
func (r *Repository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&User{}, id).Error
}

// ListRoles 列出所有角色（含权限）。
func (r *Repository) ListRoles(ctx context.Context) ([]Role, error) {
	var roles []Role
	err := r.db.WithContext(ctx).Preload("Permissions").Find(&roles).Error
	return roles, err
}

// AssignRoles 给用户分配角色（替换现有角色）。
func (r *Repository) AssignRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	var user User
	if err := r.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return err
	}

	var roles []Role
	if len(roleIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
			return err
		}
	}

	// 替换用户的角色关联
	return r.db.WithContext(ctx).Model(&user).Association("Roles").Replace(&roles)
}

// GetUserWithRoles 获取用户及其角色。
func (r *Repository) GetUserWithRoles(ctx context.Context, userID int64) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Preload("Roles").First(&user, userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
