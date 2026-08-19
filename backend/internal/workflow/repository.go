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
	return r.db.WithContext(ctx).Create(wf).Error
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

	var items []Workflow
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
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
