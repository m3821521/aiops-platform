package ai

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// AIAnalysisRecord 对应 ai_analysis 表，保存 AI 分析结果快照。
type AIAnalysisRecord struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID int64          `gorm:"index;not null" json:"incident_id"`
	ResultJSON string         `gorm:"type:longtext" json:"-"`
	Result     *AIAnalysisResult `gorm:"-" json:"result"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AIAnalysisRecord) TableName() string { return "ai_analysis" }

func (a *AIAnalysisRecord) BeforeSave(tx *gorm.DB) error {
	if a.Result != nil {
		data, err := json.Marshal(a.Result)
		if err != nil {
			return err
		}
		a.ResultJSON = string(data)
	}
	return nil
}

func (a *AIAnalysisRecord) AfterFind(tx *gorm.DB) error {
	if a.ResultJSON != "" {
		var result AIAnalysisResult
		if err := json.Unmarshal([]byte(a.ResultJSON), &result); err == nil {
			a.Result = &result
		}
	}
	return nil
}

// AIAnalysisRepository 是 AI 分析结果的 Repository。
type AIAnalysisRepository struct {
	db *gorm.DB
}

func NewAIAnalysisRepository(db *gorm.DB) *AIAnalysisRepository {
	return &AIAnalysisRepository{db: db}
}

func (r *AIAnalysisRepository) Create(ctx context.Context, record *AIAnalysisRecord) (*AIAnalysisRecord, error) {
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *AIAnalysisRepository) FindLatest(ctx context.Context, incidentID int64) (*AIAnalysisRecord, error) {
	var record AIAnalysisRecord
	err := r.db.WithContext(ctx).
		Where("incident_id = ?", incidentID).
		Order("created_at DESC").
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *AIAnalysisRepository) FindHistory(ctx context.Context, incidentID int64, limit int) ([]AIAnalysisRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	var records []AIAnalysisRecord
	err := r.db.WithContext(ctx).
		Where("incident_id = ?", incidentID).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
