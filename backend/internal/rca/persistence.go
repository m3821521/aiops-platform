package rca

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// AnalysisType 是分析类型。
type AnalysisType string

const (
	AnalysisTypeRCA AnalysisType = "rca"
)

// IncidentAnalysis 对应 incident_analysis 表，保存 RCA 分析结果快照。
type IncidentAnalysis struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID int64          `gorm:"index;not null" json:"incident_id"`
	Type       AnalysisType   `gorm:"size:32;default:rca" json:"type"`
	ResultJSON string         `gorm:"type:longtext" json:"-"`
	Result     *RCAResult     `gorm:"-" json:"result"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (IncidentAnalysis) TableName() string { return "incident_analysis" }

// BeforeSave 在保存前序列化 Result。
func (a *IncidentAnalysis) BeforeSave(tx *gorm.DB) error {
	if a.Result != nil {
		data, err := json.Marshal(a.Result)
		if err != nil {
			return err
		}
		a.ResultJSON = string(data)
	}
	return nil
}

// AfterFind 在查询后反序列化 Result。
func (a *IncidentAnalysis) AfterFind(tx *gorm.DB) error {
	if a.ResultJSON != "" {
		var result RCAResult
		if err := json.Unmarshal([]byte(a.ResultJSON), &result); err == nil {
			a.Result = &result
		}
	}
	return nil
}

// AnalysisRepository 是 incident_analysis 表的 Repository。
type AnalysisRepository struct {
	db *gorm.DB
}

func NewAnalysisRepository(db *gorm.DB) *AnalysisRepository {
	return &AnalysisRepository{db: db}
}

// Create 保存分析结果。
func (r *AnalysisRepository) Create(ctx context.Context, analysis *IncidentAnalysis) (*IncidentAnalysis, error) {
	if err := r.db.WithContext(ctx).Create(analysis).Error; err != nil {
		return nil, err
	}
	return analysis, nil
}

// FindLatest 获取指定 Incident 最近的分析结果。
func (r *AnalysisRepository) FindLatest(ctx context.Context, incidentID int64) (*IncidentAnalysis, error) {
	var analysis IncidentAnalysis
	err := r.db.WithContext(ctx).
		Where("incident_id = ?", incidentID).
		Order("created_at DESC").
		First(&analysis).Error
	if err != nil {
		return nil, err
	}
	return &analysis, nil
}

// FindHistory 获取指定 Incident 的分析历史。
func (r *AnalysisRepository) FindHistory(ctx context.Context, incidentID int64, limit int) ([]IncidentAnalysis, error) {
	if limit <= 0 {
		limit = 20
	}
	var analyses []IncidentAnalysis
	err := r.db.WithContext(ctx).
		Where("incident_id = ?", incidentID).
		Order("created_at DESC").
		Limit(limit).
		Find(&analyses).Error
	if err != nil {
		return nil, err
	}
	return analyses, nil
}
