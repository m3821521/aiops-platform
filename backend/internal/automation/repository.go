package automation

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ActionRepository 是 Action 的 Repository。
type ActionRepository struct {
	db *gorm.DB
}

func NewActionRepository(db *gorm.DB) *ActionRepository {
	return &ActionRepository{db: db}
}

func (r *ActionRepository) Create(ctx context.Context, action *Action) error {
	return r.db.WithContext(ctx).Create(action).Error
}

func (r *ActionRepository) FindByID(ctx context.Context, id int64) (*Action, error) {
	var action Action
	if err := r.db.WithContext(ctx).First(&action, id).Error; err != nil {
		return nil, err
	}
	return &action, nil
}

func (r *ActionRepository) Update(ctx context.Context, action *Action) error {
	return r.db.WithContext(ctx).Save(action).Error
}

// ListFilter 是 Action 列表的筛选条件。
type ListFilter struct {
	Status     ActionStatus
	Risk       RiskLevel
	ActionType ActionType
	UserID     int64
	IncidentID int64
	Cluster    string
}

func (r *ActionRepository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Action, int64, error) {
	query := r.db.WithContext(ctx).Model(&Action{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Risk != "" {
		query = query.Where("risk = ?", filter.Risk)
	}
	if filter.ActionType != "" {
		query = query.Where("action_type = ?", filter.ActionType)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.IncidentID > 0 {
		query = query.Where("incident_id = ?", filter.IncidentID)
	}
	if filter.Cluster != "" {
		query = query.Where("cluster = ?", filter.Cluster)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var actions []Action
	offset := (page - 1) * pageSize
	if err := query.Session(&gorm.Session{}).Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&actions).Error; err != nil {
		return nil, 0, err
	}
	return actions, total, nil
}

// FindRunningByTarget 查找同一目标是否有正在运行的 Action（用于并发控制）。
func (r *ActionRepository) FindRunningByTarget(ctx context.Context, targetType, targetName, cluster string) (*Action, error) {
	var action Action
	err := r.db.WithContext(ctx).Where(
		"target_type = ? AND target_name = ? AND cluster = ? AND status = ?",
		targetType, targetName, cluster, StatusRunning,
	).First(&action).Error
	if err != nil {
		return nil, err
	}
	return &action, nil
}

// MarkStaleRunningAsTimeout 服务启动时将所有遗留 RUNNING 状态的 Action 标记为 TIMEOUT。
// 同步执行模型下，服务重启意味着之前的 execution goroutine 已消失，无安全 Resume 能力。
// 因此启动时所有遗留 RUNNING 都视为 interrupted execution。
// 返回被更新的记录数。
func (r *ActionRepository) MarkStaleRunningAsTimeout(ctx context.Context, threshold time.Duration) (int64, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Action{}).
		Where("status = ?", StatusRunning).
		Updates(map[string]interface{}{
			"status":         StatusTimeout,
			"finished_at":    now,
			"reject_reason":  "worker crash: stale running action recovered on startup",
			"updated_at":     now,
			"lease_expires_at": nil,
		})
	return result.RowsAffected, result.Error
}

// RefreshLease 刷新正在执行的 Action 的租约过期时间。
// heartbeat goroutine 定期调用，防止任务被 Recovery Scanner 误恢复。
func (r *ActionRepository) RefreshLease(ctx context.Context, id int64, duration time.Duration) error {
	return r.db.WithContext(ctx).Model(&Action{}).
		Where("id = ? AND status = ?", id, StatusRunning).
		Update("lease_expires_at", time.Now().Add(duration)).Error
}

// RecoverExpiredLease Runtime Recovery Scanner 调用。
// 将 lease 已过期的 RUNNING Action 标记为 TIMEOUT。
// 使用 CAS 条件更新，多实例部署下只有一个实例能成功更新同一任务。
// 返回被更新的记录数。
func (r *ActionRepository) RecoverExpiredLease(ctx context.Context) (int64, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Action{}).
		Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", StatusRunning, now).
		Updates(map[string]interface{}{
			"status":          StatusTimeout,
			"finished_at":     now,
			"reject_reason":   "lease expired: worker crash recovered by runtime scanner",
			"updated_at":      now,
			"lease_expires_at": nil,
		})
	return result.RowsAffected, result.Error
}

// UpdateStatusIfRunning CAS 更新 Action 最终状态。
// 仅当当前状态为 running 时才更新，防止 Recovery 覆盖已完成的状态。
// 返回是否更新成功（RowsAffected == 1）。
func (r *ActionRepository) UpdateStatusIfRunning(ctx context.Context, id int64, newStatus ActionStatus) (bool, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Action{}).
		Where("id = ? AND status = ?", id, StatusRunning).
		Updates(map[string]interface{}{
			"status":          newStatus,
			"finished_at":     now,
			"updated_at":      now,
			"lease_expires_at": nil,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ClaimForExecution P0-05: 原子地将 Action 从 approved 声明为 running，并设置 lease。
// 使用 CAS（WHERE status='approved'）防止并发重复执行（TOCTOU 竞态）。
// 返回 (claimed bool, error)。claimed=true 表示成功获得执行权。
func (r *ActionRepository) ClaimForExecution(ctx context.Context, id int64, leaseDuration time.Duration) (bool, error) {
	now := time.Now()
	leaseExpires := now.Add(leaseDuration)
	result := r.db.WithContext(ctx).Model(&Action{}).
		Where("id = ? AND status = ?", id, StatusApproved).
		Updates(map[string]interface{}{
			"status":           StatusRunning,
			"updated_at":       now,
			"lease_expires_at": leaseExpires,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ExecutionRepository 是 ActionExecution 的 Repository。
type ExecutionRepository struct {
	db *gorm.DB
}

func NewExecutionRepository(db *gorm.DB) *ExecutionRepository {
	return &ExecutionRepository{db: db}
}

func (r *ExecutionRepository) Create(ctx context.Context, exec *ActionExecution) error {
	return r.db.WithContext(ctx).Create(exec).Error
}

func (r *ExecutionRepository) Update(ctx context.Context, exec *ActionExecution) error {
	return r.db.WithContext(ctx).Save(exec).Error
}

func (r *ExecutionRepository) ListByActionID(ctx context.Context, actionID int64) ([]ActionExecution, error) {
	var execs []ActionExecution
	if err := r.db.WithContext(ctx).Where("action_id = ?", actionID).Order("created_at DESC").Find(&execs).Error; err != nil {
		return nil, err
	}
	return execs, nil
}

// AuditRepository 是 AutomationAudit 的 Repository。
type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(ctx context.Context, audit *AutomationAudit) error {
	audit.RequestJSON = redactSensitive(audit.RequestJSON)
	audit.ResultJSON = redactSensitive(audit.ResultJSON)
	return r.db.WithContext(ctx).Create(audit).Error
}

func (r *AuditRepository) List(ctx context.Context, actionID, incidentID, userID int64, page, pageSize int) ([]AutomationAudit, int64, error) {
	query := r.db.WithContext(ctx).Model(&AutomationAudit{})
	if actionID > 0 {
		query = query.Where("action_id = ?", actionID)
	}
	if incidentID > 0 {
		query = query.Where("incident_id = ?", incidentID)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var audits []AutomationAudit
	offset := (page - 1) * pageSize
	if err := query.Session(&gorm.Session{}).Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&audits).Error; err != nil {
		return nil, 0, err
	}
	return audits, total, nil
}

// redactSensitive 脱敏敏感字段。
func redactSensitive(input string) string {
	if input == "" {
		return input
	}
	sensitiveKeys := []string{
		"password", "passwd", "token", "api_key", "apikey",
		"secret", "authorization", "kubeconfig", "kube_config",
		"private_key", "privatekey", "credential",
	}
	result := input
	for _, key := range sensitiveKeys {
		result = replaceSensitive(result, key)
	}
	return result
}

// replaceSensitive 简单替换敏感字段值。
func replaceSensitive(s, key string) string {
	// 查找 "key": " 然后替换到下一个 "
	search := `"` + key + `":"`
	idx := indexOf(s, search)
	if idx < 0 {
		// 尝试带空格的格式
		search = `"` + key + `" : "`
		idx = indexOf(s, search)
	}
	if idx < 0 {
		return s
	}
	start := idx + len(search)
	end := indexOf(s[start:], `"`)
	if end < 0 {
		return s
	}
	return s[:start] + "[REDACTED]" + s[start+end:]
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// 确保 time 被使用。
var _ = time.Now
