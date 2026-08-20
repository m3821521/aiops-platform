package workflow

import (
	"encoding/json"
	"time"
)

// WorkflowStatus 工作流状态。
type WorkflowStatus string

const (
	WorkflowStatusDraft          WorkflowStatus = "draft"
	WorkflowStatusPendingApproval WorkflowStatus = "pending_approval"
	WorkflowStatusApproved       WorkflowStatus = "approved"
	WorkflowStatusRunning        WorkflowStatus = "running"
	WorkflowStatusSuccess        WorkflowStatus = "success"
	WorkflowStatusFailed         WorkflowStatus = "failed"
	WorkflowStatusCancelled      WorkflowStatus = "cancelled"
)

// StepStatus 步骤状态。
type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusRunning StepStatus = "running"
	StepStatusSuccess StepStatus = "success"
	StepStatusFailed  StepStatus = "failed"
	StepStatusSkipped StepStatus = "skipped"
)

// StepType 步骤类型。
// 明确区分 observation/investigation/automation/verification，
// 只有 type=automation 时才允许 action_type，且 action_type 必须通过 IsSupportedActionType() 验证。
type StepType string

const (
	StepTypeObservation   StepType = "observation"
	StepTypeInvestigation StepType = "investigation"
	StepTypeAutomation    StepType = "automation"
	StepTypeVerification  StepType = "verification"
)

// ValidStepTypes 合法的步骤类型。
var ValidStepTypes = map[StepType]bool{
	StepTypeObservation:   true,
	StepTypeInvestigation: true,
	StepTypeAutomation:    true,
	StepTypeVerification:  true,
}

// IsValidStepType 检查步骤类型是否合法。
func IsValidStepType(t StepType) bool {
	return ValidStepTypes[t]
}

// IsAutomationStepType 检查是否为 automation 类型（只有 automation 类型才调用 ActionExecutor）。
func IsAutomationStepType(t StepType) bool {
	return t == StepTypeAutomation
}

// FailureStrategy 失败策略。
type FailureStrategy string

const (
	FailureStrategyStop     FailureStrategy = "stop"
	FailureStrategyContinue FailureStrategy = "continue"
)

// ValidFailureStrategies 合法的失败策略。
var ValidFailureStrategies = map[FailureStrategy]bool{
	FailureStrategyStop:     true,
	FailureStrategyContinue: true,
}

// IsValidFailureStrategy 检查失败策略是否合法。
func IsValidFailureStrategy(f FailureStrategy) bool {
	return ValidFailureStrategies[f]
}

// Workflow 自动化工作流。
type Workflow struct {
	ID          int64          `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:256;not null" json:"name"`
	Description string         `gorm:"size:512" json:"description,omitempty"`
	IncidentID  int64          `gorm:"index" json:"incident_id,omitempty"`
	Status      WorkflowStatus `gorm:"size:32;index;not null" json:"status"`
	Risk        string         `gorm:"size:32" json:"risk"`
	CreatedBy   int64          `gorm:"index" json:"created_by"`
	ApprovedBy  int64          `json:"approved_by,omitempty"`
	ApprovedAt  *time.Time     `json:"approved_at,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	DurationMs  int64          `json:"duration_ms"`
	Steps       []WorkflowStep `gorm:"foreignKey:WorkflowID" json:"steps,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (Workflow) TableName() string { return "workflows" }

// WorkflowStep 工作流步骤。
type WorkflowStep struct {
	ID              int64           `gorm:"primaryKey" json:"id"`
	WorkflowID      int64           `gorm:"index;not null" json:"workflow_id"`
	Order           int             `gorm:"not null" json:"order"`
	Name            string          `gorm:"size:256" json:"name"`
	Type            StepType        `gorm:"size:32;not null;default:automation" json:"type"` // observation/investigation/automation/verification
	ActionType      string          `gorm:"size:64" json:"action_type,omitempty"`              // 只有 type=automation 时使用
	TargetType      string          `gorm:"size:64" json:"target_type,omitempty"`
	TargetName      string          `gorm:"size:256" json:"target_name,omitempty"`
	Cluster         string          `gorm:"size:128" json:"cluster,omitempty"`
	Namespace       string          `gorm:"size:256" json:"namespace,omitempty"`
	Parameters      string          `gorm:"type:text" json:"parameters,omitempty"`
	FailureStrategy FailureStrategy `gorm:"size:16;default:stop" json:"failure_strategy"` // stop/continue
	Status          StepStatus      `gorm:"size:32;index" json:"status"`
	DependsOn       int64           `gorm:"default:0" json:"depends_on,omitempty"`
	MaxRetry        int             `gorm:"default:0" json:"max_retry"`
	RetryCount      int             `gorm:"default:0" json:"retry_count"`
	RetryDelaySec   int             `gorm:"default:5" json:"retry_delay_sec"`
	TimeoutSec      int             `gorm:"default:30" json:"timeout_sec"`
	Result          string          `gorm:"type:text" json:"result,omitempty"`
	Error           string          `json:"error,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

func (WorkflowStep) TableName() string { return "workflow_steps" }

// WorkflowExecution 工作流执行记录。
type WorkflowExecution struct {
	ID            int64          `gorm:"primaryKey" json:"id"`
	WorkflowID    int64          `gorm:"index;not null" json:"workflow_id"`
	Status        WorkflowStatus `gorm:"size:32;index;not null" json:"status"`
	TriggerType   string         `gorm:"size:32" json:"trigger_type"` // manual/incident/automation
	TriggeredBy   int64          `gorm:"index" json:"triggered_by"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
	DurationMs    int64          `json:"duration_ms"`
	Error         string         `json:"error,omitempty"`
	Result        string         `gorm:"type:text" json:"result,omitempty"`
	StepExecutions []WorkflowStepExecution `gorm:"foreignKey:WorkflowExecutionID" json:"step_executions,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (WorkflowExecution) TableName() string { return "workflow_executions" }

// WorkflowStepExecution 工作流步骤执行记录。
type WorkflowStepExecution struct {
	ID                  int64      `gorm:"primaryKey" json:"id"`
	WorkflowExecutionID int64      `gorm:"index;not null" json:"workflow_execution_id"`
	WorkflowStepID      int64      `gorm:"index;not null" json:"workflow_step_id"`
	StepName            string     `gorm:"size:256" json:"step_name"`
	StepType            StepType   `gorm:"size:32" json:"step_type"`
	ActionType          string     `gorm:"size:64" json:"action_type,omitempty"`
	Status              StepStatus `gorm:"size:32;index" json:"status"`
	Attempt             int        `gorm:"default:1" json:"attempt"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	DurationMs          int64      `json:"duration_ms"`
	Result              string     `gorm:"type:text" json:"result,omitempty"`
	Error               string     `json:"error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (WorkflowStepExecution) TableName() string { return "workflow_step_executions" }

// SetParameters 设置步骤参数。
func (s *WorkflowStep) SetParameters(params map[string]interface{}) {
	b, _ := json.Marshal(params)
	s.Parameters = string(b)
}

// GetParameters 获取步骤参数。
func (s *WorkflowStep) GetParameters() map[string]interface{} {
	if s.Parameters == "" {
		return nil
	}
	var m map[string]interface{}
	json.Unmarshal([]byte(s.Parameters), &m)
	return m
}

// CanTransition 检查工作流状态跳转是否合法。
func CanTransition(from, to WorkflowStatus) bool {
	transitions := map[WorkflowStatus][]WorkflowStatus{
		WorkflowStatusDraft:           {WorkflowStatusPendingApproval, WorkflowStatusCancelled},
		WorkflowStatusPendingApproval: {WorkflowStatusApproved, WorkflowStatusCancelled},
		WorkflowStatusApproved:        {WorkflowStatusRunning, WorkflowStatusCancelled},
		WorkflowStatusRunning:         {WorkflowStatusSuccess, WorkflowStatusFailed},
	}
	allowed, ok := transitions[from]
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
