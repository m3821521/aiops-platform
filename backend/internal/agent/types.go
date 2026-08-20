package agent

import (
	"context"
	"time"
)

// AgentType 是 Agent 的类型。
type AgentType string

const (
	// AgentTypeMonitor 监控 Agent，负责收集监控数据。
	AgentTypeMonitor AgentType = "monitor"
	// AgentTypeDiagnosis 诊断 Agent，负责根因分析。
	AgentTypeDiagnosis AgentType = "diagnosis"
	// AgentTypeExecutor 执行 Agent，负责执行运维操作。
	AgentTypeExecutor AgentType = "executor"
	// AgentTypeVerifier 验证 Agent，负责验证执行结果。
	AgentTypeVerifier AgentType = "verifier"
	// AgentTypeReporter 报告 Agent，负责生成最终报告。
	AgentTypeReporter AgentType = "reporter"
	// AgentTypeRisk 风险评估 Agent，负责评估操作风险。
	AgentTypeRisk AgentType = "risk"
)

// AgentStatus 是 Agent 的执行状态。
type AgentStatus string

const (
	AgentStatusPending   AgentStatus = "pending"
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusSuccess   AgentStatus = "success"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusSkipped   AgentStatus = "skipped"
	AgentStatusCancelled AgentStatus = "cancelled"
)

// TaskStatus 是任务的状态。
type TaskStatus string

const (
	TaskStatusPending      TaskStatus = "pending"
	TaskStatusDecomposed   TaskStatus = "decomposed"
	TaskStatusInProgress   TaskStatus = "in_progress"
	TaskStatusAwaitingApproval TaskStatus = "awaiting_approval"
	TaskStatusApproved     TaskStatus = "approved"
	TaskStatusRejected     TaskStatus = "rejected"
	TaskStatusExecuting    TaskStatus = "executing"
	TaskStatusVerifying    TaskStatus = "verifying"
	TaskStatusCompleted    TaskStatus = "completed"
	TaskStatusFailed       TaskStatus = "failed"
	TaskStatusCancelled    TaskStatus = "cancelled"
)

// RiskLevel 是风险等级。
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// Agent 是专业 Agent 的接口。
type Agent interface {
	// Name 返回 Agent 名称。
	Name() string
	// Type 返回 Agent 类型。
	Type() AgentType
	// Description 返回 Agent 描述。
	Description() string
	// Capabilities 返回 Agent 的能力列表。
	Capabilities() []string
	// Execute 执行 Agent 任务。
	Execute(ctx context.Context, task *Task) (*AgentResult, error)
}

// Task 是分配给 Agent 的任务。
type Task struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	AgentType   AgentType              `json:"agent_type"`
	AgentName   string                 `json:"agent_name,omitempty"`
	Priority    int                    `json:"priority"`
	DependsOn   []string               `json:"depends_on,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Status      AgentStatus            `json:"status"`
	Result      *AgentResult           `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
	DurationMs  int64                  `json:"duration_ms,omitempty"`
}

// AgentResult 是 Agent 执行结果。
type AgentResult struct {
	AgentName     string                 `json:"agent_name"`
	AgentType     AgentType              `json:"agent_type"`
	Summary       string                 `json:"summary"`
	Findings      []Finding              `json:"findings,omitempty"`
	Evidence      []Evidence             `json:"evidence,omitempty"`
	Recommendations []Recommendation     `json:"recommendations,omitempty"`
	RiskAssessment *RiskAssessment       `json:"risk_assessment,omitempty"`
	Metrics       map[string]interface{} `json:"metrics,omitempty"`
	RawOutput     string                 `json:"raw_output,omitempty"`
	Success       bool                   `json:"success"`
}

// Finding 是 Agent 的发现。
type Finding struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // info/warning/critical
	Resource    string `json:"resource,omitempty"`
}

// Evidence 是证据。
type Evidence struct {
	Source      string `json:"source"`
	Description string `json:"description"`
	Resource    string `json:"resource,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// Recommendation 是建议。
type Recommendation struct {
	Priority    string                 `json:"priority"` // P0/P1/P2/P3
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Reason      string                 `json:"reason"`
	Risk        RiskLevel              `json:"risk"`
	ActionType  string                 `json:"action_type,omitempty"`
	Target      string                 `json:"target,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// RiskAssessment 是风险评估。
type RiskAssessment struct {
	Level       RiskLevel `json:"level"`
	Score       float64   `json:"score"` // 0-100
	Description string    `json:"description"`
	Factors     []string  `json:"factors,omitempty"`
	RequiresApproval bool  `json:"requires_approval"`
}

// OrchestrationRequest 是编排请求。
type OrchestrationRequest struct {
	TaskID       string                 `json:"task_id,omitempty"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	IncidentID   int64                  `json:"incident_id,omitempty"`
	UserID       int64                  `json:"user_id,omitempty"`
	Username     string                 `json:"username,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	AgentTypes   []AgentType            `json:"agent_types,omitempty"` // 指定使用的 Agent 类型，为空则自动选择
	AutoApprove  bool                   `json:"auto_approve,omitempty"` // 低风险操作是否自动批准
}

// OrchestrationResult 是编排结果。
type OrchestrationResult struct {
	TaskID        string                 `json:"task_id"`
	Title         string                 `json:"title"`
	Status        TaskStatus             `json:"status"`
	Tasks         []*Task                `json:"tasks"`
	Summary       string                 `json:"summary,omitempty"`
	FinalReport   string                 `json:"final_report,omitempty"`
	RiskAssessment *RiskAssessment       `json:"risk_assessment,omitempty"`
	Recommendations []Recommendation     `json:"recommendations,omitempty"`
	Findings      []Finding              `json:"findings,omitempty"`
	Evidence      []Evidence             `json:"evidence,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	DurationMs    int64                  `json:"duration_ms,omitempty"`
}

// OrchestrationEvent 是编排过程中的事件。
type OrchestrationEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // task_started/task_completed/task_failed/approval_required/approval_granted/execution_started/verification_started/completed
	TaskID    string    `json:"task_id,omitempty"`
	AgentName string    `json:"agent_name,omitempty"`
	Message   string    `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}
