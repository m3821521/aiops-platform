package workflow

import (
	"context"

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
