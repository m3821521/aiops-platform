package automation

import (
	"encoding/json"
	"time"
)

// ActionStatus 是 Action 的状态。
type ActionStatus string

const (
	StatusProposed       ActionStatus = "proposed"
	StatusPendingApproval ActionStatus = "pending_approval"
	StatusApproved       ActionStatus = "approved"
	StatusRejected       ActionStatus = "rejected"
	StatusRunning        ActionStatus = "running"
	StatusSuccess        ActionStatus = "success"
	StatusFailed         ActionStatus = "failed"
	StatusTimeout        ActionStatus = "timeout"
	StatusCancelled      ActionStatus = "cancelled"
)

// 操作类型常量。
const (
	ActionRestartPod      = "restart_pod"
	ActionScaleDeployment = "scale_deployment"
	ActionJenkinsBuild    = "jenkins_build"
	ActionArgoCDSync      = "argocd_sync"
)

// RiskLevel 是风险等级。
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// DefaultRisk 返回操作类型的默认风险等级。
func DefaultRisk(actionType string) RiskLevel {
	switch actionType {
	case ActionRestartPod:
		return RiskMedium
	case ActionScaleDeployment, ActionJenkinsBuild, ActionArgoCDSync:
		return RiskHigh
	default:
		return RiskMedium
	}
}

// Action 是自动化操作的核心模型。
type Action struct {
	ID            int64          `gorm:"primaryKey" json:"id"`
	IncidentID    int64          `gorm:"index" json:"incident_id,omitempty"`
	UserID        int64          `gorm:"index" json:"user_id"`
	ActionType    string         `gorm:"size:32;index" json:"action_type"`
	TargetType    string         `gorm:"size:32" json:"target_type"`
	TargetName    string         `gorm:"size:256" json:"target_name"`
	Cluster       string         `gorm:"size:64;index" json:"cluster"`
	Namespace     string         `gorm:"size:128" json:"namespace"`
	Parameters    string         `gorm:"type:text" json:"parameters"` // JSON
	Reason        string         `gorm:"type:text" json:"reason"`
	Risk          RiskLevel      `gorm:"size:16" json:"risk"`
	Status        ActionStatus   `gorm:"size:32;index" json:"status"`
	ApprovedBy    int64          `json:"approved_by,omitempty"`
	ApprovedAt    *time.Time     `json:"approved_at,omitempty"`
	RejectedBy    int64          `json:"rejected_by,omitempty"`
	RejectedAt    *time.Time     `json:"rejected_at,omitempty"`
	RejectReason  string         `json:"reject_reason,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (Action) TableName() string { return "actions" }

// GetParameters 解析 Parameters JSON。
func (a *Action) GetParameters() map[string]interface{} {
	if a.Parameters == "" {
		return nil
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(a.Parameters), &params); err != nil {
		return nil
	}
	return params
}

// SetParameters 设置 Parameters JSON。
func (a *Action) SetParameters(params map[string]interface{}) {
	b, _ := json.Marshal(params)
	a.Parameters = string(b)
}

// ActionExecution 是一次执行记录。
type ActionExecution struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	ActionID     int64      `gorm:"index" json:"action_id"`
	Executor     string     `gorm:"size:32" json:"executor"`
	ExternalID   string     `gorm:"size:128" json:"external_id,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	LastPolledAt *time.Time `json:"last_polled_at,omitempty"`
	DurationMs   int64      `json:"duration_ms"`
	Status       string     `gorm:"size:32;index" json:"status"`
	Message      string     `json:"message,omitempty"`
	ResultJSON   string     `gorm:"type:text" json:"result_json,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (ActionExecution) TableName() string { return "action_executions" }

// AutomationAudit 是自动化审计记录。
type AutomationAudit struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	ActionID    int64     `gorm:"index" json:"action_id"`
	IncidentID  int64     `gorm:"index" json:"incident_id,omitempty"`
	UserID      int64     `gorm:"index" json:"user_id"`
	Operation   string    `gorm:"size:32" json:"operation"` // create/approve/reject/execute/cancel
	Target      string    `gorm:"size:256" json:"target"`
	RequestJSON string    `gorm:"type:text" json:"request_json"`
	ResultJSON  string    `gorm:"type:text" json:"result_json,omitempty"`
	Status      string    `gorm:"size:32" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (AutomationAudit) TableName() string { return "automation_audit" }

// DryRunResult 是 Dry Run 的结果。
type DryRunResult struct {
	ActionType       string `json:"action_type"`
	Target           string     `json:"target"`
	CurrentState     string     `json:"current_state"`
	ExpectedOperation string    `json:"expected_operation"`
	PotentialImpact  string     `json:"potential_impact"`
	Safe             bool       `json:"safe"`
}

// ExecutionResult 是执行结果。
type ExecutionResult struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// validTransitions 定义合法的状态跳转。
var validTransitions = map[ActionStatus][]ActionStatus{
	StatusProposed:        {StatusPendingApproval, StatusCancelled},
	StatusPendingApproval: {StatusApproved, StatusRejected, StatusCancelled},
	StatusApproved:        {StatusRunning, StatusCancelled},
	StatusRunning:         {StatusSuccess, StatusFailed, StatusTimeout},
}

// CanTransition 检查状态跳转是否合法。
func CanTransition(from, to ActionStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
