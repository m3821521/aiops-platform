package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Service 是自动化操作的核心 Service。
type Service struct {
	actions     *ActionRepository
	executions  *ExecutionRepository
	audits      *AuditRepository
	policy      *PolicyEngine
	executors   map[string]Executor
	OnExecutionComplete func(ctx context.Context, incidentID int64, action *Action, result *ExecutionResult)
}

// NewService 创建 Automation Service。
func NewService(
	actions *ActionRepository,
	executions *ExecutionRepository,
	audits *AuditRepository,
	policy *PolicyEngine,
	k8sExecutor *KubernetesExecutor,
	jenkinsExecutor *JenkinsExecutor,
	argocdExecutor *ArgoCDExecutor,
) *Service {
	executors := make(map[string]Executor)
	if k8sExecutor != nil {
		executors[ActionRestartPod] = k8sExecutor
		executors[ActionScaleDeployment] = k8sExecutor
	}
	if jenkinsExecutor != nil {
		executors[ActionJenkinsBuild] = jenkinsExecutor
	}
	if argocdExecutor != nil {
		executors[ActionArgoCDSync] = argocdExecutor
	}
	return &Service{
		actions:    actions,
		executions: executions,
		audits:     audits,
		policy:     policy,
		executors:  executors,
	}
}

// CreateAction 创建 Action Proposal。
// CreateAction 创建 Action Proposal。
// 只允许支持的 action_type: restart_pod, scale_deployment, jenkins_build, argocd_sync。
// 其他类型 (observe, investigate, restart, scale, rollback, config_change, network_check)
// 不属于 Automation，应该走 Investigation / Monitoring 流程。
func (s *Service) CreateAction(ctx context.Context, action *Action, userID int64) (*Action, error) {
	// action_type 白名单校验。
	// 禁止非法类型写入数据库，避免 DryRun/Execute 时才失败。
	if !IsSupportedActionType(action.ActionType) {
		return nil, fmt.Errorf("不支持的操作类型: %s，支持的类型: %v", action.ActionType, GetSupportedActionTypes())
	}

	action.UserID = userID
	action.Status = StatusPendingApproval
	if action.Risk == "" {
		action.Risk = DefaultRisk(action.ActionType)
	}
	action.CreatedAt = time.Now()
	action.UpdatedAt = time.Now()

	if err := s.actions.Create(ctx, action); err != nil {
		return nil, fmt.Errorf("创建 Action 失败: %w", err)
	}

	s.audit(ctx, action, "create", userID, nil)
	return action, nil
}

// Approve 审批通过。
func (s *Service) Approve(ctx context.Context, actionID, userID int64) (*Action, error) {
	action, err := s.actions.FindByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("Action 不存在: %w", err)
	}

	if err := s.policy.ValidateStateTransition(action.Status, StatusApproved); err != nil {
		return nil, err
	}

	// 四眼原则：申请人不能审批自己的操作。
	if action.UserID == userID {
		return nil, fmt.Errorf("四眼原则：申请人不能审批自己创建的操作")
	}

	now := time.Now()
	action.Status = StatusApproved
	action.ApprovedBy = userID
	action.ApprovedAt = &now
	action.UpdatedAt = now

	if err := s.actions.Update(ctx, action); err != nil {
		return nil, err
	}

	s.audit(ctx, action, "approve", userID, nil)
	return action, nil
}

// Reject 拒绝。
func (s *Service) Reject(ctx context.Context, actionID, userID int64, reason string) (*Action, error) {
	action, err := s.actions.FindByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("Action 不存在: %w", err)
	}

	if err := s.policy.ValidateStateTransition(action.Status, StatusRejected); err != nil {
		return nil, err
	}

	now := time.Now()
	action.Status = StatusRejected
	action.RejectedBy = userID
	action.RejectedAt = &now
	action.RejectReason = reason
	action.UpdatedAt = now

	if err := s.actions.Update(ctx, action); err != nil {
		return nil, err
	}

	s.audit(ctx, action, "reject", userID, map[string]interface{}{"reason": reason})
	return action, nil
}

// DryRun 执行 Dry Run。
func (s *Service) DryRun(ctx context.Context, actionID int64) (*DryRunResult, error) {
	action, err := s.actions.FindByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("Action 不存在: %w", err)
	}

	executor, ok := s.executors[action.ActionType]
	if !ok {
		return nil, fmt.Errorf("不支持的操作类型: %s", action.ActionType)
	}

	return executor.DryRun(ctx, *action)
}

// Execute 执行 Action。
func (s *Service) Execute(ctx context.Context, actionID, userID int64) (*ExecutionResult, error) {
	action, err := s.actions.FindByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("Action 不存在: %w", err)
	}

	// 状态检查：必须是 approved。
	if action.Status != StatusApproved {
		return nil, fmt.Errorf("Action 状态为 %s，只有 approved 状态才能执行", action.Status)
	}

	// 四眼原则：申请人不能执行自己创建的操作。
	if action.UserID == userID {
		return nil, fmt.Errorf("四眼原则：申请人不能执行自己创建的操作")
	}

	// 并发检查。
	if err := s.policy.CheckConcurrency(ctx, s.actions, *action); err != nil {
		return nil, err
	}

	executor, ok := s.executors[action.ActionType]
	if !ok {
		return nil, fmt.Errorf("不支持的操作类型: %s", action.ActionType)
	}

	// 先 Dry Run 验证。
	if _, err := executor.DryRun(ctx, *action); err != nil {
		return nil, fmt.Errorf("Dry Run 验证失败: %w", err)
	}

	// 创建 Execution 记录。
	exec := &ActionExecution{
		ActionID:  action.ID,
		Executor:  executor.Type(),
		StartedAt: time.Now(),
		Status:    "running",
		CreatedAt: time.Now(),
	}
	if err := s.executions.Create(ctx, exec); err != nil {
		return nil, err
	}

	// 更新 Action 状态为 running。
	action.Status = StatusRunning
	action.UpdatedAt = time.Now()
	s.actions.Update(ctx, action)

	// 执行。
	result, execErr := executor.Execute(ctx, *action)

	// 更新 Execution 记录。
	finishedAt := time.Now()
	exec.FinishedAt = &finishedAt
	exec.DurationMs = finishedAt.Sub(exec.StartedAt).Milliseconds()
	if execErr != nil {
		exec.Status = "failed"
		exec.Error = execErr.Error()
	} else if result != nil && !result.Success {
		exec.Status = "failed"
		exec.Error = result.Error
	} else {
		exec.Status = "success"
		if result != nil {
			// 执行 Verification（如果 Executor 支持）
			if verifyResult, verifyErr := executor.Verify(ctx, *action, result); verifyErr == nil && verifyResult != nil {
				// 将 Verification 结果合并到 result 中
				result.Data = map[string]interface{}{
					"verification": verifyResult,
				}
			}
			b, _ := json.Marshal(result)
			exec.ResultJSON = string(b)
		}
	}
	s.executions.Update(ctx, exec)

	// 更新 Action 状态。
	if exec.Status == "success" {
		action.Status = StatusSuccess
	} else {
		action.Status = StatusFailed
	}
	action.UpdatedAt = time.Now()
	s.actions.Update(ctx, action)

	// 审计。
	s.audit(ctx, action, "execute", userID, result)

	// Incident Timeline 集成：执行完成后写入 Incident 信号。
	if s.OnExecutionComplete != nil && action.IncidentID > 0 {
		s.OnExecutionComplete(ctx, action.IncidentID, action, result)
	}

	if execErr != nil {
		return nil, execErr
	}
	return result, nil
}

// Cancel 取消 Action。
// ExecuteWorkflowStep 由 Workflow 引擎调用，绕过审批直接执行（Workflow 已审批）。
func (s *Service) ExecuteWorkflowStep(ctx context.Context, action *Action) (*ExecutionResult, error) {
	executor, ok := s.executors[action.ActionType]
	if !ok {
		return &ExecutionResult{Success: false, Error: "不支持的操作类型: " + action.ActionType}, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return executor.Execute(execCtx, *action)
}

func (s *Service) Cancel(ctx context.Context, actionID, userID int64) (*Action, error) {
	action, err := s.actions.FindByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("Action 不存在: %w", err)
	}

	if err := s.policy.ValidateStateTransition(action.Status, StatusCancelled); err != nil {
		return nil, err
	}

	action.Status = StatusCancelled
	action.UpdatedAt = time.Now()
	if err := s.actions.Update(ctx, action); err != nil {
		return nil, err
	}

	s.audit(ctx, action, "cancel", userID, nil)
	return action, nil
}

// GetAction 获取 Action 详情。
func (s *Service) GetAction(ctx context.Context, actionID int64) (*Action, error) {
	return s.actions.FindByID(ctx, actionID)
}

// ListActions 获取 Action 列表。
func (s *Service) ListActions(ctx context.Context, filter ListFilter, page, pageSize int) ([]Action, int64, error) {
	return s.actions.List(ctx, filter, page, pageSize)
}

// ListExecutions 获取 Execution 历史。
func (s *Service) ListExecutions(ctx context.Context, actionID int64) ([]ActionExecution, error) {
	return s.executions.ListByActionID(ctx, actionID)
}

// ListAudit 获取审计日志。
func (s *Service) ListAudit(ctx context.Context, actionID, incidentID, userID int64, page, pageSize int) ([]AutomationAudit, int64, error) {
	return s.audits.List(ctx, actionID, incidentID, userID, page, pageSize)
}

// audit 记录审计日志。
func (s *Service) audit(ctx context.Context, action *Action, operation string, userID int64, result interface{}) {
	reqJSON, _ := json.Marshal(map[string]interface{}{
		"action_type": action.ActionType,
		"target":      action.TargetName,
		"namespace":   action.Namespace,
		"cluster":     action.Cluster,
		"parameters":  action.GetParameters(),
		"reason":      action.Reason,
		"risk":        action.Risk,
	})
	resJSON := ""
	if result != nil {
		b, _ := json.Marshal(result)
		resJSON = string(b)
	}
	_ = s.audits.Create(ctx, &AutomationAudit{
		ActionID:    action.ID,
		IncidentID:  action.IncidentID,
		UserID:      userID,
		Operation:   operation,
		Target:      fmt.Sprintf("%s/%s", action.TargetType, action.TargetName),
		RequestJSON: string(reqJSON),
		ResultJSON:  resJSON,
		Status:      string(action.Status),
		CreatedAt:   time.Now(),
	})
}
