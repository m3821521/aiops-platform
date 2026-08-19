package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"gorm.io/gorm"
)

// ToolAuditRecord 是 ai_tool_audit 表的模型。
type ToolAuditRecord struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	RequestID     string    `gorm:"index;size:64" json:"request_id"`
	IncidentID    int64     `gorm:"index" json:"incident_id"`
	UserID        int64     `gorm:"index" json:"user_id"`
	ToolName      string    `gorm:"size:64;index" json:"tool_name"`
	InputJSON     string    `gorm:"type:text" json:"input_json"`
	OutputSummary string    `gorm:"type:text" json:"output_summary"`
	Success       bool      `json:"success"`
	Available     bool      `json:"available"`
	DurationMs    int64     `json:"duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

func (ToolAuditRecord) TableName() string { return "ai_tool_audit" }

// ToolAuditRepository 是 Tool 审计的 Repository。
type ToolAuditRepository struct {
	db *gorm.DB
}

func NewToolAuditRepository(db *gorm.DB) *ToolAuditRepository {
	return &ToolAuditRepository{db: db}
}

// Create 保存一条 Tool 审计记录（自动脱敏）。
func (r *ToolAuditRepository) Create(ctx context.Context, record *ToolAuditRecord) error {
	record.InputJSON = redactSensitive(record.InputJSON)
	record.OutputSummary = redactSensitive(record.OutputSummary)
	return r.db.WithContext(ctx).Create(record).Error
}

// List 分页查询 Tool 审计记录。
func (r *ToolAuditRepository) List(ctx context.Context, filter ToolAuditFilter, page, pageSize int) ([]ToolAuditRecord, int64, error) {
	query := r.db.WithContext(ctx).Model(&ToolAuditRecord{})
	if filter.IncidentID > 0 {
		query = query.Where("incident_id = ?", filter.IncidentID)
	}
	if filter.ToolName != "" {
		query = query.Where("tool_name = ?", filter.ToolName)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if !filter.Start.IsZero() {
		query = query.Where("created_at >= ?", filter.Start)
	}
	if !filter.End.IsZero() {
		query = query.Where("created_at <= ?", filter.End)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []ToolAuditRecord
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// ToolAuditFilter 是审计查询的筛选条件。
type ToolAuditFilter struct {
	IncidentID int64
	ToolName   string
	UserID     int64
	Start      time.Time
	End        time.Time
}

// redactSensitive 对 JSON 字符串中的敏感字段进行脱敏。
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
		// 匹配 "key": "value" 模式（大小写不敏感）。
		pattern := `(?i)"` + key + `"\s*:\s*"[^"]*"`
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, `"`+key+`":"[REDACTED]"`)
	}
	return result
}

// RecordFromToolCall 从 ToolCall 创建审计记录。
func RecordFromToolCall(call ToolCall, requestID string, incidentID, userID int64) *ToolAuditRecord {
	inputJSON, _ := json.Marshal(call.Input)
	outputSummary := ""
	if call.Result.Data != nil {
		// 只保存摘要，不保存完整数据。
		summary := map[string]interface{}{
			"success":   call.Result.Success,
			"available": call.Result.Available,
			"source":    call.Result.Source,
		}
		if call.Result.Error != "" {
			summary["error"] = call.Result.Error
		}
		b, _ := json.Marshal(summary)
		outputSummary = string(b)
	}
	return &ToolAuditRecord{
		RequestID:     requestID,
		IncidentID:    incidentID,
		UserID:        userID,
		ToolName:      call.ToolName,
		InputJSON:     string(inputJSON),
		OutputSummary: outputSummary,
		Success:       call.Result.Success,
		Available:     call.Result.Available,
		DurationMs:    call.Duration.Milliseconds(),
		CreatedAt:     call.Timestamp,
	}
}
