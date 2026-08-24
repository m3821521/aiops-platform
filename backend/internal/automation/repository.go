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

// MarkStaleRunningAsTimeout 服务启动时将遗留 RUNNING 状态的 Action 标记为 TIMEOUT。
// P0-05.2: 只恢复 lease 已过期，或无 lease 且 updated_at 超过 threshold 的 Action。
// 正常 heartbeat 的 Action（lease 未过期）禁止被 recovery 误杀。
// 返回被更新的记录数。
func (r *ActionRepository) MarkStaleRunningAsTimeout(ctx context.Context, threshold time.Duration) (int64, error) {
	now := time.Now()
	cutoff := now.Add(-threshold)
	// P0-05.3: 显式括号，不依赖 SQL AND/OR precedence。
	// 条件：
	//   status = running
	//   AND (
	//     (lease_expires_at IS NOT NULL AND lease_expires_at < NOW())
	//     OR
	//     (lease_expires_at IS NULL AND updated_at < cutoff)
	//   )
	result := r.db.WithContext(ctx).Model(&Action{}).
		Where("status = ? AND ((lease_expires_at IS NOT NULL AND lease_expires_at < ?) OR (lease_expires_at IS NULL AND updated_at < ?))",
			StatusRunning, now, cutoff).
		Updates(map[string]interface{}{
			"status":          StatusTimeout,
			"finished_at":     now,
			"reject_reason":   "worker crash: stale running action recovered on startup",
			"updated_at":      now,
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

// ClaimForExecution P0-05 + P0-05.2 + P0-05.4: 原子地将 Action 从 approved 声明为 running，并设置 lease。
// 使用 CAS（WHERE status='approved'）防止并发重复执行（TOCTOU 竞态）。
// 同时使用 NOT EXISTS 子查询原子检查同一 target 是否有其他 running action，防止资源并发。
// P0-05.4: MySQL Error 1093 不允许在 UPDATE 的 FROM 子句中直接引用正在更新的表。
// 使用子查询别名包装（NOT EXISTS (SELECT 1 FROM (SELECT ... FROM actions a2) AS sub)）绕过此限制。
// 此方案在 MySQL 和 SQLite 上均兼容，且在多连接并发下通过行锁保证原子性。
// 返回 (claimed bool, error)。claimed=true 表示成功获得执行权。
func (r *ActionRepository) ClaimForExecution(ctx context.Context, id int64, leaseDuration time.Duration) (bool, error) {
	now := time.Now()
	leaseExpires := now.Add(leaseDuration)
	// 原子 CAS + 资源并发检查（NOT EXISTS + 子查询别名包装，MySQL/SQLite 兼容）：
	//   1. 当前 action 状态必须为 approved
	//   2. 同一 cluster+target_type+target_name 不能有其他 running action
	result := r.db.WithContext(ctx).Exec(`
		UPDATE actions 
		SET status=?, updated_at=?, lease_expires_at=?
		WHERE id=? AND status=? AND NOT EXISTS (
			SELECT 1 FROM (
				SELECT 1 FROM actions a2 
				WHERE a2.target_type = actions.target_type 
				  AND a2.target_name = actions.target_name 
				  AND a2.cluster = actions.cluster 
				  AND a2.status = ? 
				  AND a2.id != actions.id
			) AS sub
		)
	`, StatusRunning, now, leaseExpires, id, StatusApproved, StatusRunning)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// RollbackClaim P0-05.3: Execution 创建失败时，用 CAS 将 Action 从 running 回滚到 approved。
// 仅当当前状态为 running 时才回滚，防止覆盖其他状态。
// 返回是否回滚成功（RowsAffected == 1）。
func (r *ActionRepository) RollbackClaim(ctx context.Context, id int64) (bool, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Action{}).
		Where("id = ? AND status = ?", id, StatusRunning).
		Updates(map[string]interface{}{
			"status":           StatusApproved,
			"updated_at":       now,
			"lease_expires_at": nil,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// HasRunningByTarget 检查同一 target 是否有正在运行的 Action（用于 Claim 失败后区分原因）。
func (r *ActionRepository) HasRunningByTarget(ctx context.Context, targetType, targetName, cluster string, excludeID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Action{}).
		Where("target_type = ? AND target_name = ? AND cluster = ? AND status = ? AND id != ?",
			targetType, targetName, cluster, StatusRunning, excludeID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
