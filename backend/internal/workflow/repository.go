package workflow

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type ListFilter struct {
	Status     WorkflowStatus
	IncidentID int64
	CreatedBy  int64
}

func (r *Repository) Create(ctx context.Context, wf *Workflow) error {
	// Omit("Steps") 避免 GORM 自动创建关联的 Steps，Steps 由 CreateWorkflow 方法手动创建
	return r.db.WithContext(ctx).Omit("Steps").Create(wf).Error
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Workflow, error) {
	var wf Workflow
	if err := r.db.WithContext(ctx).Preload("Steps").First(&wf, id).Error; err != nil {
		return nil, err
	}
	return &wf, nil
}

func (r *Repository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Workflow, int64, error) {
	query := r.db.WithContext(ctx).Model(&Workflow{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.IncidentID > 0 {
		query = query.Where("incident_id = ?", filter.IncidentID)
	}
	if filter.CreatedBy > 0 {
		query = query.Where("created_by = ?", filter.CreatedBy)
	}

	var total int64
	query.Count(&total)

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var items []Workflow
	offset := (page - 1) * pageSize
	if err := query.Session(&gorm.Session{}).Order("id DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *Repository) Update(ctx context.Context, wf *Workflow) error {
	return r.db.WithContext(ctx).Save(wf).Error
}

func (r *Repository) UpdateStatus(ctx context.Context, id int64, status WorkflowStatus) error {
	return r.db.WithContext(ctx).Model(&Workflow{}).Where("id = ?", id).Update("status", status).Error
}

// FindStaleRunning 查找超过 threshold 时间仍处于 RUNNING 状态的 Workflow（服务崩溃恢复用）。
func (r *Repository) FindStaleRunning(ctx context.Context, threshold time.Duration) ([]Workflow, error) {
	cutoff := time.Now().Add(-threshold)
	var items []Workflow
	if err := r.db.WithContext(ctx).Where("status = ? AND updated_at < ?", WorkflowStatusRunning, cutoff).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// MarkStaleRunningAsFailed 服务启动时将所有遗留 RUNNING 状态的 Workflow 标记为 FAILED。
// 同步执行模型下，服务重启意味着之前的 execution goroutine 已消失，无安全 Resume 能力。
// 因此启动时所有遗留 RUNNING 都视为 interrupted execution。
// 返回被更新的记录数。
func (r *Repository) MarkStaleRunningAsFailed(ctx context.Context, threshold time.Duration) (int64, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Workflow{}).
		Where("status = ?", WorkflowStatusRunning).
		Updates(map[string]interface{}{
			"status":           WorkflowStatusFailed,
			"finished_at":      now,
			"error":            "worker crash: stale running workflow recovered on startup",
			"updated_at":       now,
			"lease_expires_at": nil,
		})
	return result.RowsAffected, result.Error
}

// RefreshLease 刷新正在执行的 Workflow 的租约过期时间。
// heartbeat goroutine 定期调用，覆盖整个 Workflow 执行周期（包括 Step 之间和 retry delay）。
func (r *Repository) RefreshLease(ctx context.Context, id int64, duration time.Duration) error {
	return r.db.WithContext(ctx).Model(&Workflow{}).
		Where("id = ? AND status = ?", id, WorkflowStatusRunning).
		Update("lease_expires_at", time.Now().Add(duration)).Error
}

// RecoverExpiredLease Runtime Recovery Scanner 调用。
// 将 lease 已过期的 RUNNING Workflow 标记为 FAILED。
// 使用 CAS 条件更新，多实例部署下只有一个实例能成功更新同一任务。
// 返回被更新的记录数。
func (r *Repository) RecoverExpiredLease(ctx context.Context) (int64, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Workflow{}).
		Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", WorkflowStatusRunning, now).
		Updates(map[string]interface{}{
			"status":           WorkflowStatusFailed,
			"finished_at":      now,
			"error":            "lease expired: worker crash recovered by runtime scanner",
			"updated_at":       now,
			"lease_expires_at": nil,
		})
	return result.RowsAffected, result.Error
}

// UpdateStatusIfRunning CAS 更新 Workflow 最终状态。
// 仅当当前状态为 running 时才更新，防止 Recovery 覆盖已完成的状态。
// 返回是否更新成功（RowsAffected == 1）。
func (r *Repository) UpdateStatusIfRunning(ctx context.Context, id int64, newStatus WorkflowStatus) (bool, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&Workflow{}).
		Where("id = ? AND status = ?", id, WorkflowStatusRunning).
		Updates(map[string]interface{}{
			"status":           newStatus,
			"finished_at":      now,
			"updated_at":       now,
			"lease_expires_at": nil,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ClaimForExecution P0-04: 原子地将 Workflow 从 approved 声明为 running，并设置 lease。
// 使用 CAS（WHERE status='approved'）防止并发重复执行（TOCTOU 竞态）。
// 返回 (claimed bool, error)。claimed=true 表示成功获得执行权。
func (r *Repository) ClaimForExecution(ctx context.Context, id int64, leaseDuration time.Duration) (bool, error) {
	now := time.Now()
	leaseExpires := now.Add(leaseDuration)
	result := r.db.WithContext(ctx).Model(&Workflow{}).
		Where("id = ? AND status = ?", id, WorkflowStatusApproved).
		Updates(map[string]interface{}{
			"status":           WorkflowStatusRunning,
			"started_at":       now,
			"updated_at":       now,
			"lease_expires_at": leaseExpires,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *Repository) CreateStep(ctx context.Context, step *WorkflowStep) error {
	return r.db.WithContext(ctx).Create(step).Error
}

func (r *Repository) UpdateStep(ctx context.Context, step *WorkflowStep) error {
	return r.db.WithContext(ctx).Save(step).Error
}

func (r *Repository) ListSteps(ctx context.Context, workflowID int64) ([]WorkflowStep, error) {
	var steps []WorkflowStep
	if err := r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("\"order\" ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	return steps, nil
}

// ==================== WorkflowExecution ====================

// CreateExecution 创建工作流执行记录。
func (r *Repository) CreateExecution(ctx context.Context, exec *WorkflowExecution) error {
	return r.db.WithContext(ctx).Create(exec).Error
}

// FindExecutionByID 根据 ID 查找工作流执行记录。
func (r *Repository) FindExecutionByID(ctx context.Context, id int64) (*WorkflowExecution, error) {
	var exec WorkflowExecution
	if err := r.db.WithContext(ctx).Preload("StepExecutions").First(&exec, id).Error; err != nil {
		return nil, err
	}
	return &exec, nil
}

// ListExecutionsByWorkflowID 列出某个工作流的所有执行记录。
func (r *Repository) ListExecutionsByWorkflowID(ctx context.Context, workflowID int64, page, pageSize int) ([]WorkflowExecution, int64, error) {
	query := r.db.WithContext(ctx).Model(&WorkflowExecution{}).Where("workflow_id = ?", workflowID)

	var total int64
	query.Count(&total)

	// Count 会污染 query 对象，必须使用 Session 创建新会话
	var items []WorkflowExecution
	offset := (page - 1) * pageSize
	if err := query.Session(&gorm.Session{}).Order("id DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// UpdateExecution 更新工作流执行记录。
func (r *Repository) UpdateExecution(ctx context.Context, exec *WorkflowExecution) error {
	return r.db.WithContext(ctx).Save(exec).Error
}

// ==================== WorkflowStepExecution ====================

// CreateStepExecution 创建步骤执行记录。
func (r *Repository) CreateStepExecution(ctx context.Context, stepExec *WorkflowStepExecution) error {
	return r.db.WithContext(ctx).Create(stepExec).Error
}

// UpdateStepExecution 更新步骤执行记录。
func (r *Repository) UpdateStepExecution(ctx context.Context, stepExec *WorkflowStepExecution) error {
	return r.db.WithContext(ctx).Save(stepExec).Error
}

// ListStepExecutions 列出某个工作流执行的所有步骤执行记录。
func (r *Repository) ListStepExecutions(ctx context.Context, workflowExecutionID int64) ([]WorkflowStepExecution, error) {
	var items []WorkflowStepExecution
	if err := r.db.WithContext(ctx).Where("workflow_execution_id = ?", workflowExecutionID).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
