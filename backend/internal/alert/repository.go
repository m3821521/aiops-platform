package alert

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository 封装 alerts 表的数据库操作。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建告警存储库。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListFilter 是告警列表的过滤条件。
type ListFilter struct {
	Status    string
	Severity  string
	Alertname string
	Namespace string
	Service   string
}

// Upsert 按 fingerprint 创建或更新告警。
// Alertmanager 重复推送同一条告警时，更新最新状态而非创建重复记录。
func (r *Repository) Upsert(ctx context.Context, a *Alert) (*Alert, error) {
	if a.Fingerprint == "" {
		return nil, errors.New("fingerprint 不能为空")
	}

	var existing Alert
	err := r.db.WithContext(ctx).Where("fingerprint = ?", a.Fingerprint).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if existing.ID > 0 {
		// 更新已有记录：保留 ID 和创建时间，更新其他字段。
		a.ID = existing.ID
		a.CreatedAt = existing.CreatedAt
		if err := r.db.WithContext(ctx).Save(a).Error; err != nil {
			return nil, err
		}
		return a, nil
	}

	// 新建。
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// FindByID 按 ID 查询告警。
func (r *Repository) FindByID(ctx context.Context, id int64) (*Alert, error) {
	var a Alert
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, err
	}
	// 补充关联的 Incident IDs。
	if err := r.fillIncidentIDs(ctx, []*Alert{&a}); err != nil {
		// 关联查询失败不影响主数据返回，仅记录空。
		a.IncidentIDs = nil
	}
	return &a, nil
}

// FindByFingerprint 按指纹查询告警。
func (r *Repository) FindByFingerprint(ctx context.Context, fp string) (*Alert, error) {
	var a Alert
	if err := r.db.WithContext(ctx).Where("fingerprint = ?", fp).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// List 分页查询告警，按 starts_at 倒序。返回告警列表和总数。
func (r *Repository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Alert, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	q := r.db.WithContext(ctx).Model(&Alert{})
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	if filter.Alertname != "" {
		q = q.Where("alertname = ?", filter.Alertname)
	}
	if filter.Namespace != "" {
		q = q.Where("namespace = ?", filter.Namespace)
	}
	if filter.Service != "" {
		q = q.Where("service = ?", filter.Service)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Count 会污染 query 对象，必须使用 Session 创建新会话，否则 Find 返回空。
	var alerts []Alert
	offset := (page - 1) * pageSize
	if err := q.Session(&gorm.Session{}).Order("starts_at DESC").Offset(offset).Limit(pageSize).Find(&alerts).Error; err != nil {
		return nil, 0, err
	}
	// 批量补充关联的 Incident IDs（避免 N+1）。
	if len(alerts) > 0 {
		ptrs := make([]*Alert, len(alerts))
		for i := range alerts {
			ptrs[i] = &alerts[i]
		}
		if err := r.fillIncidentIDs(ctx, ptrs); err != nil {
			// 关联查询失败不影响主数据返回。
		}
	}
	return alerts, total, nil
}

// fillIncidentIDs 批量查询 Alert 关联的 Incident IDs。
// 通过 incident_signals 表关联：Alert.Fingerprint = IncidentSignal.SignalID (signal_type='alert')。
// 使用 IN (...) 批量查询，避免 N+1。一个 Alert 可能关联多个 Incident。
func (r *Repository) fillIncidentIDs(ctx context.Context, alerts []*Alert) error {
	if len(alerts) == 0 {
		return nil
	}
	fingerprints := make([]string, 0, len(alerts))
	fpSet := make(map[string]bool, len(alerts))
	for _, a := range alerts {
		if a.Fingerprint != "" && !fpSet[a.Fingerprint] {
			fingerprints = append(fingerprints, a.Fingerprint)
			fpSet[a.Fingerprint] = true
		}
	}
	if len(fingerprints) == 0 {
		return nil
	}

	// 批量查询 incident_signals：signal_type='alert' AND signal_id IN (...)
	type signalRow struct {
		SignalID   string
		IncidentID int64
	}
	var rows []signalRow
	if err := r.db.WithContext(ctx).Table("incident_signals").
		Select("signal_id, incident_id").
		Where("signal_type = ? AND signal_id IN ?", "alert", fingerprints).
		Find(&rows).Error; err != nil {
		return err
	}

	// 按 signal_id 分组。
	fpToIncidents := make(map[string][]int64, len(fingerprints))
	for _, row := range rows {
		fpToIncidents[row.SignalID] = append(fpToIncidents[row.SignalID], row.IncidentID)
	}

	// 映射到 Alert。
	for _, a := range alerts {
		if ids, ok := fpToIncidents[a.Fingerprint]; ok && len(ids) > 0 {
			a.IncidentIDs = ids
		}
	}
	return nil
}

// UpdateStatus 更新告警状态。
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.db.WithContext(ctx).Model(&Alert{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": status, "updated_at": time.Now()}).Error
}

// Acknowledge 确认告警，状态改为 acknowledged。
func (r *Repository) Acknowledge(ctx context.Context, id int64) error {
	return r.UpdateStatus(ctx, id, StatusAcknowledged)
}

// Resolve 关闭告警，状态改为 resolved 并填写结束时间。
func (r *Repository) Resolve(ctx context.Context, id int64, endsAt time.Time) error {
	return r.db.WithContext(ctx).Model(&Alert{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     StatusResolved,
			"ends_at":    endsAt,
			"updated_at": time.Now(),
		}).Error
}
