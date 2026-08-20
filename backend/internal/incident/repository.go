package incident

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository 封装 incidents 和 incident_signals 表的数据库操作。
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListFilter 是 Incident 列表的过滤条件。
type ListFilter struct {
	Status    string
	Severity  string
	Namespace string
	Service   string
	Cluster   string
	Keyword   string
	StartTime *time.Time
	EndTime   *time.Time
}

// Create 创建新 Incident。
func (r *Repository) Create(ctx context.Context, inc *Incident) (*Incident, error) {
	if err := r.db.WithContext(ctx).Create(inc).Error; err != nil {
		return nil, err
	}
	return inc, nil
}

// FindByID 按 ID 查询 Incident（含 Signals）。
func (r *Repository) FindByID(ctx context.Context, id int64) (*Incident, error) {
	var inc Incident
	if err := r.db.WithContext(ctx).Preload("Signals", func(db *gorm.DB) *gorm.DB {
		return db.Order("timestamp ASC")
	}).First(&inc, id).Error; err != nil {
		return nil, err
	}
	return &inc, nil
}

// List 分页查询 Incident，按 start_time 倒序。
func (r *Repository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Incident, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	q := r.db.WithContext(ctx).Model(&Incident{})
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Severity != "" {
		q = q.Where("severity = ?", filter.Severity)
	}
	if filter.Namespace != "" {
		q = q.Where("namespace = ?", filter.Namespace)
	}
	if filter.Service != "" {
		q = q.Where("service = ?", filter.Service)
	}
	if filter.Cluster != "" {
		q = q.Where("cluster = ?", filter.Cluster)
	}
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		q = q.Where("title LIKE ? OR summary LIKE ? OR root_cause LIKE ?", like, like, like)
	}
	if filter.StartTime != nil {
		q = q.Where("start_time >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		q = q.Where("start_time <= ?", *filter.EndTime)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var incidents []Incident
	offset := (page - 1) * pageSize
	if err := q.Session(&gorm.Session{}).Order("start_time DESC").Offset(offset).Limit(pageSize).Find(&incidents).Error; err != nil {
		return nil, 0, err
	}
	return incidents, total, nil
}

// UpdateStatus 更新 Incident 状态。
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status string) error {
	updates := map[string]any{"status": status, "updated_at": time.Now()}
	if status == StatusResolved || status == StatusClosed {
		now := time.Now()
		updates["end_time"] = &now
	}
	return r.db.WithContext(ctx).Model(&Incident{}).Where("id = ?", id).Updates(updates).Error
}

// Update 更新 Incident 字段。
func (r *Repository) Update(ctx context.Context, inc *Incident) error {
	return r.db.WithContext(ctx).Save(inc).Error
}

// FindActiveByResource 查询指定资源的活跃 Incident（open/acknowledged）。
// 用于 Correlation Engine 查找可关联的现有 Incident。
func (r *Repository) FindActiveByResource(ctx context.Context, cluster, namespace, service string, since time.Time) ([]Incident, error) {
	q := r.db.WithContext(ctx).Model(&Incident{}).
		Where("status IN ?", []string{StatusOpen, StatusAcknowledged}).
		Where("start_time >= ?", since)
	if cluster != "" {
		q = q.Where("cluster = ?", cluster)
	}
	// namespace 和 service 不强制过滤，因为关联引擎会评分；
	// 但为了减少候选集，同 namespace 或同 service 优先。
	if namespace != "" {
		q = q.Where("(namespace = ? OR service = ? OR namespace = '' OR service = '')", namespace, service)
	}
	var incidents []Incident
	if err := q.Order("start_time DESC").Limit(50).Find(&incidents).Error; err != nil {
		return nil, err
	}
	return incidents, nil
}

// --- Signal 操作 ---

// UpsertSignal 按 (incident_id, signal_type, signal_id) 创建或更新信号。
// 同一信号重复到达时更新状态（如 resolved），不创建重复记录。
func (r *Repository) UpsertSignal(ctx context.Context, sig *IncidentSignal) (*IncidentSignal, error) {
	if sig.SignalID == "" {
		return nil, errors.New("signal_id 不能为空")
	}
	var existing IncidentSignal
	err := r.db.WithContext(ctx).Where("incident_id = ? AND signal_type = ? AND signal_id = ?",
		sig.IncidentID, sig.SignalType, sig.SignalID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing.ID > 0 {
		sig.ID = existing.ID
		sig.CreatedAt = existing.CreatedAt
		if err := r.db.WithContext(ctx).Save(sig).Error; err != nil {
			return nil, err
		}
		return sig, nil
	}
	if err := r.db.WithContext(ctx).Create(sig).Error; err != nil {
		return nil, err
	}
	return sig, nil
}

// FindSignalByExternalID 按外部信号 ID 查找信号。
func (r *Repository) FindSignalByExternalID(ctx context.Context, signalType, signalID string) (*IncidentSignal, error) {
	var sig IncidentSignal
	if err := r.db.WithContext(ctx).Where("signal_type = ? AND signal_id = ?", signalType, signalID).
		Order("id DESC").First(&sig).Error; err != nil {
		return nil, err
	}
	return &sig, nil
}

// ListSignals 查询 Incident 的所有信号，按时间排序。
func (r *Repository) ListSignals(ctx context.Context, incidentID int64) ([]IncidentSignal, error) {
	var signals []IncidentSignal
	if err := r.db.WithContext(ctx).Where("incident_id = ?", incidentID).
		Order("timestamp ASC").Find(&signals).Error; err != nil {
		return nil, err
	}
	return signals, nil
}

// CountActiveSignals 统计 Incident 中未 resolved 的信号数。
func (r *Repository) CountActiveSignals(ctx context.Context, incidentID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&IncidentSignal{}).
		Where("incident_id = ? AND resolved = ?", incidentID, false).Count(&count).Error
	return count, err
}

// IncrementSignalCount 增加 Incident 的 signal_count。
func (r *Repository) IncrementSignalCount(ctx context.Context, incidentID int64) error {
	return r.db.WithContext(ctx).Model(&Incident{}).Where("id = ?", incidentID).
		UpdateColumn("signal_count", gorm.Expr("signal_count + 1")).Error
}
