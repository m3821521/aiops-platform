package audit

import (
	"context"

	"gorm.io/gorm"
)

// Repository 审计日志数据访问层。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建审计日志 Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 记录一条审计日志。
func (r *Repository) Create(ctx context.Context, log *Log) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListFilter 审计日志查询过滤条件。
type ListFilter struct {
	UserID   *int64
	Username string
	Action   string
	Resource string
	Result   string
}

// List 分页查询审计日志。
func (r *Repository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&Log{})

	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.Username != "" {
		query = query.Where("username = ?", filter.Username)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Resource != "" {
		query = query.Where("resource = ?", filter.Resource)
	}
	if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}

	var total int64
	query.Count(&total)

	var logs []Log
	err := query.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
