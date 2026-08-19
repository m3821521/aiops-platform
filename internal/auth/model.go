package auth

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型。
type User struct {
	ID           int64          `gorm:"primaryKey" json:"id"`
	Username     string         `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string         `gorm:"size:255;not null" json:"-"`
	Email        string         `gorm:"size:128" json:"email"`
	FullName     string         `gorm:"size:128" json:"full_name"`
	Status       string         `gorm:"size:16;default:active" json:"status"` // active / disabled
	LastLoginAt  *time.Time     `json:"last_login_at"`
	Roles        []Role         `gorm:"many2many:user_roles;" json:"roles,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Role 角色模型。
type Role struct {
	ID          int64        `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"uniqueIndex;size:64;not null" json:"name"` // admin / operator / viewer
	Description string       `gorm:"size:255" json:"description"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Permission 权限模型。
type Permission struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;size:128;not null" json:"name"` // 如 alerts:read
	Resource    string    `gorm:"size:64;not null;index" json:"resource"`    // 如 alerts
	Action      string    `gorm:"size:16;not null" json:"action"`            // read / write
	Description string    `gorm:"size:255" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// HasPermission 检查用户是否拥有指定权限。
func (u *User) HasPermission(resource, action string) bool {
	required := resource + ":" + action
	for _, role := range u.Roles {
		if role.Name == "admin" {
			return true // admin 拥有所有权限
		}
		for _, perm := range role.Permissions {
			if perm.Name == required {
				return true
			}
		}
	}
	return false
}

// RoleNames 返回用户的角色名列表。
func (u *User) RoleNames() []string {
	names := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		names = append(names, r.Name)
	}
	return names
}
