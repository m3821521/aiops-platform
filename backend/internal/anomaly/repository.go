package anomaly

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository 封装 anomaly_records 表的数据库操作。
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListFilter 是异常记录列表的过滤条件。
type ListFilter struct {
	Cluster      string
	Namespace    string
	ResourceType string
	ResourceName string
	Severity     string
	Algorithm    string
	Status       string
	Metric       string
	StartTime    *time.Time
	EndTime      *time.Time
}

// Create 创建异常记录。
func (r *Repository) Create(ctx context.Context, rec *AnomalyRecord) (*AnomalyRecord, error) {
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// FindByID 按 ID 查询。
func (r *Repository) FindByID(ctx context.Context, id int64) (*AnomalyRecord, error) {
	var rec AnomalyRecord
	if err := r.db.WithContext(ctx).First(&rec, id).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// FindByIdentity 按唯一标识查找（用于幂等检查）。
// 唯一标识：cluster + resource_type + resource_name + metric + timestamp + algorithm。
func (r *Repository) FindByIdentity(ctx context.Context, rec *AnomalyRecord) (*AnomalyRecord, error) {
	var existing AnomalyRecord
	err := r.db.WithContext(ctx).Where(
		"cluster = ? AND resource_type = ? AND resource_name = ? AND metric = ? AND timestamp = ? AND algorithm = ?",
		rec.Cluster, rec.ResourceType, rec.ResourceName, rec.Metric, rec.Timestamp, rec.Algorithm,
	).First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// Upsert 按唯一标识创建或更新。幂等写入。
func (r *Repository) Upsert(ctx context.Context, rec *AnomalyRecord) (*AnomalyRecord, bool, error) {
	existing, err := r.FindByIdentity(ctx, rec)
	if err == nil && existing.ID > 0 {
		// 已存在，更新状态和值。
		existing.Value = rec.Value
		existing.AnomalyScore = rec.AnomalyScore
		existing.Severity = rec.Severity
		existing.Reason = rec.Reason
		existing.Status = rec.Status
		if rec.ResolvedAt != nil {
			existing.ResolvedAt = rec.ResolvedAt
		}
		if err := r.db.WithContext(ctx).Save(existing).Error; err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	// 新建。
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, false, err
	}
	return rec, true, nil
}

// List 分页查询，按 timestamp 倒序。
func (r *Repository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]AnomalyRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	q := r.db.WithContext(ctx).Model(&AnomalyRecord{})
	q = applyFilter(q, filter)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var records []AnomalyRecord
	offset := (page - 1) * pageSize
	if err := q.Session(&gorm.Session{}).Order("timestamp DESC").Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// FindByResource 按资源查询异常记录。
func (r *Repository) FindByResource(ctx context.Context, cluster, resourceType, resourceName string, since time.Time) ([]AnomalyRecord, error) {
	var records []AnomalyRecord
	q := r.db.WithContext(ctx).Model(&AnomalyRecord{}).
		Where("timestamp >= ?", since)
	if cluster != "" {
		q = q.Where("cluster = ?", cluster)
	}
	if resourceType != "" {
		q = q.Where("resource_type = ?", resourceType)
	}
	if resourceName != "" {
		q = q.Where("resource_name = ?", resourceName)
	}
	if err := q.Order("timestamp DESC").Limit(100).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// FindByTimeRange 按时间范围查询。
func (r *Repository) FindByTimeRange(ctx context.Context, start, end time.Time, filter ListFilter) ([]AnomalyRecord, error) {
	var records []AnomalyRecord
	q := r.db.WithContext(ctx).Model(&AnomalyRecord{}).
		Where("timestamp >= ? AND timestamp <= ?", start, end)
	q = applyFilter(q, filter)
	if err := q.Order("timestamp ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// FindActive 查询活跃异常（status != resolved）。
func (r *Repository) FindActive(ctx context.Context, filter ListFilter) ([]AnomalyRecord, error) {
	var records []AnomalyRecord
	q := r.db.WithContext(ctx).Model(&AnomalyRecord{}).
		Where("status != ?", AnomalyStatusResolved)
	q = applyFilter(q, filter)
	if err := q.Order("timestamp DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// CountActive 统计活跃异常数量。
func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&AnomalyRecord{}).
		Where("status != ?", AnomalyStatusResolved).Count(&count).Error
	return count, err
}

// UpdateStatus 更新异常状态。
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status string) error {
	updates := map[string]any{"status": status, "updated_at": time.Now()}
	if status == AnomalyStatusResolved {
		now := time.Now()
		updates["resolved_at"] = &now
	}
	return r.db.WithContext(ctx).Model(&AnomalyRecord{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateIncident 关联 Incident。
func (r *Repository) UpdateIncident(ctx context.Context, id, incidentID int64) error {
	return r.db.WithContext(ctx).Model(&AnomalyRecord{}).Where("id = ?", id).
		Update("incident_id", incidentID).Error
}

// FindActiveByResourceAndMetric 查询某资源+metric的活跃异常。
// 用于去重合并和异常恢复：同一资源+metric在短时间内应合并为一条持续异常。
func (r *Repository) FindActiveByResourceAndMetric(ctx context.Context, cluster, resourceType, resourceName, metric string) (*AnomalyRecord, error) {
	var rec AnomalyRecord
	err := r.db.WithContext(ctx).Model(&AnomalyRecord{}).
		Where("cluster = ? AND resource_type = ? AND resource_name = ? AND metric = ? AND status != ?",
			cluster, resourceType, resourceName, metric, AnomalyStatusResolved).
		Order("timestamp DESC").
		First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// MarkResolved 标记异常为已恢复。
func (r *Repository) MarkResolved(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&AnomalyRecord{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":     AnomalyStatusResolved,
			"resolved_at": &now,
			"updated_at": now,
		}).Error
}

// applyFilter 应用通用过滤条件。
func applyFilter(q *gorm.DB, filter ListFilter) *gorm.DB {
	if filter.Cluster != "" {
		q = q.Where("cluster = ?", filter.Cluster)
	}
	if filter.Namespace != "" {
		q = q.Where("namespace = ?", filter.Namespace)
	}
	if filter.ResourceType != "" {
		q = q.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.ResourceName != "" {
		q = q.Where("resource_name = ?", filter.ResourceName)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	if filter.Algorithm != "" {
		q = q.Where("algorithm = ?", filter.Algorithm)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Metric != "" {
		q = q.Where("metric LIKE ?", "%"+filter.Metric+"%")
	}
	if filter.StartTime != nil {
		q = q.Where("timestamp >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		q = q.Where("timestamp <= ?", *filter.EndTime)
	}
	return q
}
