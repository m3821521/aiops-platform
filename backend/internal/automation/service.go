package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"
)

// Lease / Heartbeat 配置。
const (
	heartbeatInterval  = 20 * time.Second
	leaseTimeout       = 60 * time.Second
	recoveryScanInterval = 30 * time.Second
	recoveryJitter     = 5 * time.Second
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

// RecoverStaleActions 服务启动时恢复因 Worker 崩溃而永久停留在 RUNNING 状态的 Action。
// 将超过 threshold 时间未更新的 RUNNING Action 标记为 TIMEOUT，避免状态永久卡住。
// 返回被恢复的 Action 数量。
func (s *Service) RecoverStaleActions(ctx context.Context, threshold time.Duration) (int64, error) {
	if threshold <= 0 {
		threshold = 5 * time.Minute
	}
	count, err := s.actions.MarkStaleRunningAsTimeout(ctx, threshold)
	if err != nil {
		slog.Error("recover stale actions failed", "error", err)
		return 0, err
	}
	if count > 0 {
		slog.Warn("recovered stale actions", "count", count, "threshold", threshold.String())
	}
	return count, nil
}

// StartRecoveryScanner 启动后台 Runtime Recovery Scanner。
// 每约 30 秒（带 ±5 秒 jitter）扫描 lease 已过期的 RUNNING Action，标记为 TIMEOUT。
// 每个服务实例都可以独立运行，CAS 条件更新保证多实例下不重复处理。
// 调用方应在服务启动时调用，并在优雅关闭时 cancel context。
func (s *Service) StartRecoveryScanner(ctx context.Context) {
	go func() {
		slog.Info("action recovery scanner started", "interval", recoveryScanInterval.String(), "jitter", recoveryJitter.String())
		for {
			// 带 jitter 的等待，避免多实例同时扫描造成 DB 压力尖峰。
			jitter := time.Duration(rand.Int63n(int64(recoveryJitter*2))) - recoveryJitter
			wait := recoveryScanInterval + jitter
			select {
			case <-ctx.Done():
				slog.Info("action recovery scanner stopped")
				return
			case <-time.After(wait):
				count, err := s.actions.RecoverExpiredLease(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return // context 被取消，正常退出
					}
					slog.Error("action recovery scanner failed", "error", err)
				} else if count > 0 {
					slog.Warn("recovered expired lease actions", "count", count)
				}
			}
		}
	}()
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

	// 并发检查（快速失败预检查，最终原子保证由 ClaimForExecution 的 NOT EXISTS 子查询提供）。
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

	// P0-05 + P0-05.2: 原子 Claim，防止 TOCTOU 竞态导致重复执行。
	// 使用 CAS (WHERE status='approved') + NOT EXISTS 子查询（同一 target 无其他 running）。
	// P0-05.2: 先 Claim 再创建 Execution，避免 100 并发产生 100 个 orphan execution 记录。
	claimed, err := s.actions.ClaimForExecution(ctx, actionID, leaseTimeout)
	if err != nil {
		return nil, fmt.Errorf("claim action for execution failed: %w", err)
	}
	if !claimed {
		// 区分失败原因：已被其他请求 claim vs 同一 target 有其他 running action
		current, _ := s.actions.FindByID(ctx, actionID)
		if current != nil && current.Status != StatusApproved {
			return nil, fmt.Errorf("Action 已被其他请求声明执行或状态已变更（当前状态: %s），无法重复执行", current.Status)
		}
		// 状态仍为 approved，说明是 target 并发冲突
		hasRunning, _ := s.actions.HasRunningByTarget(ctx, action.TargetType, action.TargetName, action.Cluster, action.ID)
		if hasRunning {
			return nil, fmt.Errorf("RESOURCE_BUSY: 目标资源 %s/%s 已有其他操作正在执行", action.TargetType, action.TargetName)
		}
		return nil, fmt.Errorf("Action 声明执行失败（可能是并发冲突），请重试")
	}

	// Claim 成功后重新读取 action，获取更新后的状态和 lease。
	action, err = s.actions.FindByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf("reload action after claim failed: %w", err)
	}

	// P0-05.2: Claim 成功后才创建 Execution 记录，避免 orphan execution。
	exec := &ActionExecution{
		ActionID:  action.ID,
		Executor:  executor.Type(),
		StartedAt: time.Now(),
		Status:    "running",
		CreatedAt: time.Now(),
	}
	if err := s.executions.Create(ctx, exec); err != nil {
		// P0-05.3: Execution 创建失败，用 CAS 将 Action 从 running 回滚到 approved。
		// 不能使用简单的 Update(action)，因为 action 已经是 RUNNING 状态。
		rolledBack, rbErr := s.actions.RollbackClaim(ctx, actionID)
		if rbErr != nil {
			slog.Error("rollback claim after execution create failure failed",
				"action_id", actionID, "error", rbErr)
		} else if !rolledBack {
			slog.Warn("rollback claim skipped (action status changed by another process)",
				"action_id", actionID)
		}
		return nil, fmt.Errorf("创建 execution 记录失败: %w", err)
	}

	// 启动 heartbeat goroutine，定期刷新 lease。
	// 使用独立 context，heartbeat 失败时取消执行 context，阻止 executor 继续执行。
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	go func() {
		defer heartbeatCancel()
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := s.actions.RefreshLease(heartbeatCtx, action.ID, leaseTimeout); err != nil {
					slog.Error("action heartbeat refresh failed", "action_id", action.ID, "error", err)
					// heartbeat 持续失败，触发执行 context cancellation
					// 注意：这里不直接 cancel ctx，因为 executor 可能正在执行关键操作
					// 记录错误即可，lease 过期后 Recovery Scanner 会处理
				}
			}
		}
	}()

	// 执行。
	result, execErr := executor.Execute(ctx, *action)

	// 停止 heartbeat。
	heartbeatCancel()

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

	// CAS 更新 Action 最终状态。
	// 仅当当前状态为 running 时才更新，防止 Recovery Scanner 覆盖已完成的状态。
	finalStatus := StatusSuccess
	if exec.Status != "success" {
		finalStatus = StatusFailed
	}
	updated, err := s.actions.UpdateStatusIfRunning(ctx, action.ID, finalStatus)
	if err != nil {
		slog.Error("action final status update failed", "action_id", action.ID, "error", err)
	} else if !updated {
		// RowsAffected == 0，说明状态已被 Recovery 修改
		slog.Warn("action status already changed by recovery, skipping final update",
			"action_id", action.ID, "expected_status", finalStatus)
		// 读取当前状态用于诊断
		if current, ferr := s.actions.FindByID(ctx, action.ID); ferr == nil {
			action = current
		}
	} else {
		action.Status = finalStatus
	}

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
