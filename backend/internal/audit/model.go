package audit

import "time"

// Log 审计日志记录。
type Log struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	UserID       *int64    `json:"user_id,omitempty"`
	Username     string    `gorm:"size:64" json:"username"`
	Action       string    `gorm:"size:64;not null;index" json:"action"`
	Resource     string    `gorm:"size:64;not null;index" json:"resource"`
	ResourceID   string    `gorm:"size:128" json:"resource_id"`
	Request      string    `gorm:"type:text" json:"request,omitempty"`
	Result       string    `gorm:"size:16;default:success" json:"result"`
	ErrorMessage string    `gorm:"type:text" json:"error_message,omitempty"`
	IP           string    `gorm:"size:45" json:"ip"`
	UserAgent    string    `gorm:"size:255" json:"user_agent"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名。
func (Log) TableName() string {
	return "audit_logs"
}
