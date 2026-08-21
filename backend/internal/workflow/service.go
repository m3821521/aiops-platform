package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Lease / Heartbeat 配置。
const (
	wfHeartbeatInterval   = 20 * time.Second
	wfLeaseTimeout        = 60 * time.Second
	wfRecoveryScanInterval = 30 * time.Second
	wfRecoveryJitter      = 5 * time.Second
)

// ActionExecutor 执行单个 Action 的接口。
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, actionType, cluster, namespace, targetName string, params map[string]interface{}) (success bool, message string, err error)
}

// K8sQuerier 用于 observation/investigation/verification 步骤的只读 Kubernetes 查询。
// 只允许查询，不允许修改。
type K8sQuerier interface {
	GetPod(ctx context.Context, cluster, namespace, name string) (*corev1.Pod, error)
	GetPodEvents(ctx context.Context, cluster, namespace, pod string) ([]corev1.Event, error)
	ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error)
}

// DryRunResult Dry Run 结果。
type DryRunResult struct {
	Valid    bool                 `json:"valid"`
	Workflow *Workflow            `json:"workflow"`
	Steps    []DryRunStepResult  `json:"steps"`
	Message  string               `json:"message"`
}

// DryRunStepResult 单个步骤的 Dry Run 结果。
type DryRunStepResult struct {
	StepID       int64    `json:"step_id"`
	StepName     string   `json:"step_name"`
	StepType     StepType `json:"step_type"`
	ActionType   string   `json:"action_type,omitempty"`
	Valid        bool     `json:"valid"`
	Message      string   `json:"message"`
	CanExecute   bool     `json:"can_execute"`
	RetryEnabled bool     `json:"retry_enabled"`
	MaxRetry     int      `json:"max_retry"`
	RetryDelaySec int     `json:"retry_delay_sec"`
	Backoff      string   `json:"backoff"`
	MaxDelaySec  int      `json:"max_delay_sec"`
	FailureStrategy string `json:"failure_strategy"`
}

// PodObservationResult observation 步骤的 Pod 观察结果。
type PodObservationResult struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	UID         string `json:"uid"`
	Status      string `json:"status"`
	RestartCount int32 `json:"restart_count"`
	NodeName    string `json:"node_name"`
	PodIP       string `json:"pod_ip"`
}

type Service struct {
	repo      *Repository
	executor  ActionExecutor
	k8sQuerier K8sQuerier
}

func NewService(repo *Repository, executor ActionExecutor) *Service {
	return &Service{repo: repo, executor: executor}
}

// SetK8sQuerier 设置 Kubernetes 查询器（用于 observation/investigation/verification 步骤）。
func (s *Service) SetK8sQuerier(querier K8sQuerier) {
	s.k8sQuerier = querier
}

// RecoverStaleExecutions 服务启动时恢复因 Worker 崩溃而永久停留在 RUNNING 状态的 Workflow。
// 将超过 threshold 时间未更新的 RUNNING Workflow 标记为 FAILED，避免状态永久卡住。
// 返回被恢复的 Workflow 数量。
func (s *Service) RecoverStaleExecutions(ctx context.Context, threshold time.Duration) (int64, error) {
	if threshold <= 0 {
		threshold = 5 * time.Minute
	}
	count, err := s.repo.MarkStaleRunningAsFailed(ctx, threshold)
	if err != nil {
		slog.Error("recover stale workflow executions failed", "error", err)
		return 0, err
	}
	if count > 0 {
		slog.Warn("recovered stale workflow executions", "count", count, "threshold", threshold.String())
	}
	return count, nil
}

// StartRecoveryScanner 启动后台 Runtime Recovery Scanner。
// 每约 30 秒（带 ±5 秒 jitter）扫描 lease 已过期的 RUNNING Workflow，标记为 FAILED。
// 每个服务实例都可以独立运行，CAS 条件更新保证多实例下不重复处理。
// 调用方应在服务启动时调用，并在优雅关闭时 cancel context。
func (s *Service) StartRecoveryScanner(ctx context.Context) {
	go func() {
		slog.Info("workflow recovery scanner started", "interval", wfRecoveryScanInterval.String(), "jitter", wfRecoveryJitter.String())
		for {
			// 带 jitter 的等待，避免多实例同时扫描造成 DB 压力尖峰。
			jitter := time.Duration(rand.Int63n(int64(wfRecoveryJitter*2))) - wfRecoveryJitter
			wait := wfRecoveryScanInterval + jitter
			select {
			case <-ctx.Done():
				slog.Info("workflow recovery scanner stopped")
				return
			case <-time.After(wait):
				count, err := s.repo.RecoverExpiredLease(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return // context 被取消，正常退出
					}
					slog.Error("workflow recovery scanner failed", "error", err)
				} else if count > 0 {
					slog.Warn("recovered expired lease workflows", "count", count)
				}
			}
		}
	}()
}

// maxRetryDelay 最大重试延迟（秒）。
const maxRetryDelay = 30

// calculateRetryDelay 计算 exponential backoff 重试延迟。
// delay = min(RetryDelaySec * 2^(attempt-1), maxRetryDelay)
// attempt 是当前失败的 attempt（从1开始）。
func calculateRetryDelay(retryDelaySec, attempt int) time.Duration {
	if retryDelaySec <= 0 {
		retryDelaySec = 5
	}
	delay := retryDelaySec
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > maxRetryDelay {
			delay = maxRetryDelay
			break
		}
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	return time.Duration(delay) * time.Second
}

// waitWithContext context-aware 等待。
// 如果 context 被取消，立即返回错误。
func waitWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// checkAutomationRetrySafety 检查 automation 步骤是否可以安全重试。
// 对于 restart_pod：如果旧 Pod 已经不存在，说明第一次操作可能已经成功，
// 不应再次删除新 Pod。
// 返回 canRetry bool, message string, err error。
func (s *Service) checkAutomationRetrySafety(ctx context.Context, step *WorkflowStep, attempt int) (bool, string, error) {
	if step.Type != StepTypeAutomation {
		return true, "", nil
	}

	// 只有 restart_pod 需要特殊安全检查
	if step.ActionType != "restart_pod" {
		return true, "", nil
	}

	if s.k8sQuerier == nil {
		return false, "Kubernetes 查询器未配置，无法安全重试 automation", fmt.Errorf("K8sQuerier not configured")
	}

	// 查询旧 Pod 是否还存在
	oldPod, err := s.k8sQuerier.GetPod(ctx, step.Cluster, step.Namespace, step.TargetName)
	if err != nil {
		// 旧 Pod 不存在，说明第一次 restart 可能已经成功
		// 尝试通过 ListPods 找到新 Pod
		pods, listErr := s.k8sQuerier.ListPods(ctx, step.Cluster, step.Namespace)
		if listErr != nil {
			return false, "无法确认 automation 操作状态，拒绝安全重试", fmt.Errorf("cannot verify automation state: %v", listErr)
		}
		// 找到状态为 Running 的 Pod（假设是新重建的 Pod）
		for _, p := range pods {
			if p.Status.Phase == "Running" {
				return false, fmt.Sprintf("检测到旧 Pod 已不存在，新 Pod %s 已 Running，第一次 restart 很可能已成功，拒绝重复操作", p.Name),
					fmt.Errorf("operation already completed: old pod not found, new pod %s is running", p.Name)
			}
		}
		// 旧 Pod 不存在且没有 Running 的新 Pod，状态不明确
		return false, "旧 Pod 已不存在但未找到新 Running Pod，操作状态不明确，拒绝安全重试",
			fmt.Errorf("operation state is ambiguous: old pod not found, no running pod")
	}

	// 旧 Pod 仍然存在，说明第一次 restart 没有生效，可以安全重试
	_ = oldPod
	return true, fmt.Sprintf("旧 Pod %s 仍然存在，可以安全重试", step.TargetName), nil
}

// ValidateWorkflow 验证工作流结构。
func (s *Service) ValidateWorkflow(wf *Workflow) error {
	if wf.Name == "" {
		return fmt.Errorf("工作流名称不能为空")
	}
	if len(wf.Steps) == 0 {
		return fmt.Errorf("工作流至少需要一个步骤")
	}
	for i, step := range wf.Steps {
		if step.Name == "" {
			return fmt.Errorf("步骤 %d 名称不能为空", i+1)
		}
		if !IsValidStepType(step.Type) {
			return fmt.Errorf("步骤 %d 类型不合法: %s", i+1, step.Type)
		}
		// 只有 automation 类型才需要 action_type
		if IsAutomationStepType(step.Type) {
			if step.ActionType == "" {
				return fmt.Errorf("步骤 %d (automation) action_type 不能为空", i+1)
			}
		}
		if !IsValidFailureStrategy(step.FailureStrategy) {
			return fmt.Errorf("步骤 %d 失败策略不合法: %s", i+1, step.FailureStrategy)
		}
	}
	return nil
}

// CreateWorkflow 创建工作流（draft 状态）。
func (s *Service) CreateWorkflow(ctx context.Context, wf *Workflow, userID int64) (*Workflow, error) {
	// 验证工作流结构
	if err := s.ValidateWorkflow(wf); err != nil {
		return nil, err
	}
	wf.Status = WorkflowStatusDraft
	wf.CreatedBy = userID
	wf.CreatedAt = time.Now()
	wf.UpdatedAt = time.Now()
	if wf.Risk == "" {
		wf.Risk = "high"
	}
	if err := s.repo.Create(ctx, wf); err != nil {
		return nil, err
	}
	// 创建步骤
	for i := range wf.Steps {
		wf.Steps[i].ID = 0 // 清空 ID，让数据库自动分配
		wf.Steps[i].WorkflowID = wf.ID
		wf.Steps[i].Status = StepStatusPending
		if wf.Steps[i].FailureStrategy == "" {
			wf.Steps[i].FailureStrategy = FailureStrategyStop
		}
		wf.Steps[i].CreatedAt = time.Now()
		wf.Steps[i].UpdatedAt = time.Now()
		if err := s.repo.CreateStep(ctx, &wf.Steps[i]); err != nil {
			return nil, err
		}
	}
	return wf, nil
}

// Submit 提交审批。
func (s *Service) Submit(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}
	if !CanTransition(wf.Status, WorkflowStatusPendingApproval) {
		return nil, fmt.Errorf("状态 %s 不能提交审批", wf.Status)
	}
	wf.Status = WorkflowStatusPendingApproval
	wf.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, wf); err != nil {
		return nil, err
	}
	return wf, nil
}

// Approve 审批通过。
func (s *Service) Approve(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}
	if !CanTransition(wf.Status, WorkflowStatusApproved) {
		return nil, fmt.Errorf("状态 %s 不能审批", wf.Status)
	}
	// 四眼原则
	if wf.CreatedBy == userID {
		return nil, fmt.Errorf("四眼原则：申请人不能审批自己创建的工作流")
	}
	now := time.Now()
	wf.Status = WorkflowStatusApproved
	wf.ApprovedBy = userID
	wf.ApprovedAt = &now
	wf.UpdatedAt = now
	if err := s.repo.Update(ctx, wf); err != nil {
		return nil, err
	}
	return wf, nil
}

// DryRun 模拟执行工作流，不真正修改任何资源。
func (s *Service) DryRun(ctx context.Context, workflowID int64) (*DryRunResult, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}

	result := &DryRunResult{
		Workflow: wf,
		Valid:    true,
		Steps:    make([]DryRunStepResult, 0, len(wf.Steps)),
	}

	steps := wf.Steps
	sort.Slice(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })

	for _, step := range steps {
		stepResult := DryRunStepResult{
			StepID:          step.ID,
			StepName:        step.Name,
			StepType:        step.Type,
			ActionType:      step.ActionType,
			Valid:           true,
			CanExecute:      true,
			RetryEnabled:    step.MaxRetry > 0,
			MaxRetry:        step.MaxRetry,
			RetryDelaySec:   step.RetryDelaySec,
			Backoff:         "exponential",
			MaxDelaySec:     maxRetryDelay,
			FailureStrategy: string(step.FailureStrategy),
		}

		// 验证步骤类型
		if !IsValidStepType(step.Type) {
			stepResult.Valid = false
			stepResult.CanExecute = false
			stepResult.Message = fmt.Sprintf("不合法的步骤类型: %s", step.Type)
			result.Valid = false
			result.Steps = append(result.Steps, stepResult)
			continue
		}

		// 根据步骤类型处理
		switch step.Type {
		case StepTypeObservation:
			stepResult.Message = "观察步骤：将查询系统状态，不修改任何资源"
		case StepTypeInvestigation:
			stepResult.Message = "调查步骤：将进行故障分析，不修改任何资源"
		case StepTypeAutomation:
			if step.ActionType == "" {
				stepResult.Valid = false
				stepResult.CanExecute = false
				stepResult.Message = "automation 步骤缺少 action_type"
				result.Valid = false
			} else if s.executor == nil {
				stepResult.Valid = false
				stepResult.CanExecute = false
				stepResult.Message = "Action executor 未配置"
				result.Valid = false
			} else {
				stepResult.Message = fmt.Sprintf("自动化步骤：将执行 %s，目标: %s/%s", step.ActionType, step.Namespace, step.TargetName)
			}
		case StepTypeVerification:
			stepResult.Message = "验证步骤：将验证前序步骤的执行结果"
		}

		result.Steps = append(result.Steps, stepResult)
	}

	if result.Valid {
		result.Message = fmt.Sprintf("工作流 %s Dry Run 通过，共 %d 个步骤", wf.Name, len(steps))
	} else {
		result.Message = fmt.Sprintf("工作流 %s Dry Run 未通过", wf.Name)
	}

	return result, nil
}

// Execute 执行工作流（按顺序执行步骤，支持依赖、失败策略、执行记录）。
func (s *Service) Execute(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}
	if wf.Status != WorkflowStatusApproved {
		return nil, fmt.Errorf("只有 approved 状态才能执行，当前: %s", wf.Status)
	}
	// 四眼原则：创建者不能执行自己的工作流
	if wf.CreatedBy == userID {
		return nil, fmt.Errorf("四眼原则：申请人不能执行自己创建的工作流")
	}
	// 并发保护：防止 running 状态重复执行
	if wf.Status == WorkflowStatusRunning {
		return nil, fmt.Errorf("Workflow 正在执行中，不能重复执行")
	}

	now := time.Now()
	wf.Status = WorkflowStatusRunning
	wf.StartedAt = &now
	wf.UpdatedAt = now
	leaseExpires := now.Add(wfLeaseTimeout)
	wf.LeaseExpiresAt = &leaseExpires
	s.repo.Update(ctx, wf)

	// 创建工作流执行记录
	exec := &WorkflowExecution{
		WorkflowID:  wf.ID,
		Status:      WorkflowStatusRunning,
		TriggerType: "manual",
		TriggeredBy: userID,
		StartedAt:   &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateExecution(ctx, exec); err != nil {
		// 执行记录创建失败不影响工作流执行，但记录错误
		slog.Error("创建工作流执行记录失败", "workflow_id", wf.ID, "error", err)
	}

	// 启动 heartbeat goroutine，定期刷新 lease。
	// heartbeat 覆盖整个 Workflow 执行周期，包括 Step 之间、retry、retry delay。
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	go func() {
		defer heartbeatCancel()
		ticker := time.NewTicker(wfHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := s.repo.RefreshLease(heartbeatCtx, wf.ID, wfLeaseTimeout); err != nil {
					slog.Error("workflow heartbeat refresh failed", "workflow_id", wf.ID, "error", err)
				}
			}
		}
	}()

	steps := wf.Steps
	sort.Slice(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })

	stepResults := make(map[int64]bool)
	allSuccess := true
	// 执行上下文：保存 observation 步骤的 Pod UID，供 verification 步骤使用
	execContext := make(map[string]interface{})

	for i := range steps {
		step := &steps[i]

		// 检查依赖
		if step.DependsOn > 0 {
			if !stepResults[step.DependsOn] {
				step.Status = StepStatusSkipped
				step.UpdatedAt = time.Now()
				s.repo.UpdateStep(ctx, step)
				// 记录步骤执行
				s.recordStepExecution(ctx, exec.ID, step, StepStatusSkipped, "依赖步骤未成功，跳过", nil)
				continue
			}
		}

		// 执行步骤（带 Retry Loop）
		step.Status = StepStatusRunning
		step.UpdatedAt = time.Now()
		s.repo.UpdateStep(ctx, step)

		maxAttempts := step.MaxRetry + 1
		var stepSuccess bool
		var stepMessage string
		var stepErr error
		step.RetryCount = 0

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			// automation retry 安全检查（第2次及以后的尝试）
			if attempt > 1 && step.Type == StepTypeAutomation {
				canRetry, safetyMsg, safetyErr := s.checkAutomationRetrySafety(ctx, step, attempt)
				if !canRetry {
					// 无法安全重试，记录最后一次失败并退出
					stepStarted := time.Now()
					stepFinished := time.Now()
					step.StartedAt = &stepStarted
					step.FinishedAt = &stepFinished
					s.recordStepExecutionWithAttempt(ctx, exec.ID, step, StepStatusFailed, safetyMsg, safetyErr, attempt, &stepStarted, &stepFinished)
					stepSuccess = false
					stepMessage = safetyMsg
					stepErr = safetyErr
					break
				}
				// 可以安全重试，记录安全检查日志
				slog.Info("workflow step automation retry safety check passed",
					"workflow_id", wf.ID, "execution_id", exec.ID, "step_id", step.ID,
					"attempt", attempt, "message", safetyMsg)
			}

			// 执行当前 attempt
			stepStarted := time.Now()

			// 创建 Step Timeout Context（每个 attempt 独立计算 timeout）
			// Retry Delay 不计入 Step Timeout，因此使用原始 ctx 作为 parent
			timeoutSec := step.TimeoutSec
			if timeoutSec <= 0 {
				timeoutSec = 30 // 默认 30 秒，与 gorm default 一致
			}
			stepCtx, stepCancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)

			var success bool
			var message string
			var execErr error

			switch step.Type {
			case StepTypeObservation:
				success, message, execErr = s.executeObservation(stepCtx, step, execContext)
			case StepTypeInvestigation:
				success, message, execErr = s.executeInvestigation(stepCtx, step)
			case StepTypeAutomation:
				if s.executor == nil {
					success = false
					message = "Action executor 未配置"
					execErr = fmt.Errorf("Action executor 未配置")
				} else {
					success, message, execErr = s.executor.ExecuteAction(
						stepCtx, step.ActionType, step.Cluster, step.Namespace, step.TargetName, step.GetParameters(),
					)
				}
			case StepTypeVerification:
				success, message, execErr = s.executeVerification(stepCtx, step, execContext)
			default:
				success = false
				message = fmt.Sprintf("不支持的步骤类型: %s", step.Type)
				execErr = fmt.Errorf("不支持的步骤类型: %s", step.Type)
			}

			stepFinished := time.Now()
			step.StartedAt = &stepStarted
			step.FinishedAt = &stepFinished

			// 区分 Step Timeout 和 Workflow Cancel
			// 如果原始 ctx 已取消 → Workflow Cancel（不是 timeout）
			// 如果 stepCtx 超时但原始 ctx 未取消 → Step Timeout
			if !success && execErr != nil {
				if ctx.Err() != nil {
					// Workflow 被取消，保留原始错误语义
					execErr = fmt.Errorf("workflow cancelled: %w", ctx.Err())
					message = fmt.Sprintf("Workflow 被取消: %v", ctx.Err())
				} else if stepCtx.Err() == context.DeadlineExceeded {
					// Step Timeout
					execErr = fmt.Errorf("workflow step timeout: exceeded %d seconds", timeoutSec)
					message = fmt.Sprintf("workflow step timeout: 超过 %d 秒", timeoutSec)
				}
			}
			stepCancel()

			if success {
				// 成功，记录本次 attempt 并退出 Retry Loop
				s.recordStepExecutionWithAttempt(ctx, exec.ID, step, StepStatusSuccess, message, execErr, attempt, &stepStarted, &stepFinished)
				stepSuccess = true
				stepMessage = message
				stepErr = nil
				break
			}

			// 失败，记录本次 attempt
			s.recordStepExecutionWithAttempt(ctx, exec.ID, step, StepStatusFailed, message, execErr, attempt, &stepStarted, &stepFinished)

			// 如果已经是最后一次尝试，退出 Retry Loop
			if attempt >= maxAttempts {
				// 全部失败时 RetryCount = MaxRetry = maxAttempts - 1
				step.RetryCount = maxAttempts - 1
				stepSuccess = false
				stepMessage = message
				stepErr = execErr
				break
			}

			// 不是最后一次，更新 RetryCount 并准备重试
			step.RetryCount = attempt

			// 计算 retry delay 并等待
			retryDelay := calculateRetryDelay(step.RetryDelaySec, attempt)
			slog.Warn("workflow step failed, retrying",
				"workflow_id", wf.ID, "execution_id", exec.ID, "step_id", step.ID,
				"step_type", step.Type, "attempt", attempt, "max_attempts", maxAttempts,
				"retry_count", step.RetryCount, "retry_delay", retryDelay.String(), "error", execErr)

			if waitErr := waitWithContext(ctx, retryDelay); waitErr != nil {
				// context 被取消，立即退出
				stepSuccess = false
				stepMessage = fmt.Sprintf("Workflow 被取消: %v", waitErr)
				stepErr = waitErr
				break
			}
		}

		// 更新 Step 最终状态
		step.Result = stepMessage
		if stepErr != nil {
			step.Error = stepErr.Error()
		}

		if stepSuccess {
			step.Status = StepStatusSuccess
			stepResults[step.ID] = true
		} else {
			step.Status = StepStatusFailed
			stepResults[step.ID] = false
			allSuccess = false

			// 根据失败策略处理
			if step.FailureStrategy == FailureStrategyStop || step.FailureStrategy == "" {
				// 后续步骤跳过
				for j := i + 1; j < len(steps); j++ {
					steps[j].Status = StepStatusSkipped
					steps[j].UpdatedAt = time.Now()
					s.repo.UpdateStep(ctx, &steps[j])
					s.recordStepExecution(ctx, exec.ID, &steps[j], StepStatusSkipped, "前序步骤失败，停止执行", nil)
				}
				break
			}
			// FailureStrategyContinue: 继续执行后续步骤
		}
		step.UpdatedAt = time.Now()
		s.repo.UpdateStep(ctx, step)
	}

	// 停止 heartbeat。
	heartbeatCancel()

	finished := time.Now()
	wf.FinishedAt = &finished
	wf.DurationMs = finished.Sub(now).Milliseconds()
	finalStatus := WorkflowStatusSuccess
	if !allSuccess {
		finalStatus = WorkflowStatusFailed
	}

	// CAS 更新 Workflow 最终状态。
	// 仅当当前状态为 running 时才更新，防止 Recovery Scanner 覆盖已完成的状态。
	updated, err := s.repo.UpdateStatusIfRunning(ctx, wf.ID, finalStatus)
	if err != nil {
		slog.Error("workflow final status update failed", "workflow_id", wf.ID, "error", err)
	} else if !updated {
		// RowsAffected == 0，说明状态已被 Recovery 修改
		slog.Warn("workflow status already changed by recovery, skipping final update",
			"workflow_id", wf.ID, "expected_status", finalStatus)
		// 读取当前状态用于诊断
		if current, ferr := s.repo.FindByID(ctx, wf.ID); ferr == nil {
			wf = current
		}
	} else {
		wf.Status = finalStatus
		wf.UpdatedAt = finished
	}

	// 更新工作流执行记录
	exec.Status = wf.Status
	exec.FinishedAt = &finished
	exec.DurationMs = wf.DurationMs
	if !allSuccess {
		exec.Error = "工作流执行失败"
	}
	exec.UpdatedAt = finished
	s.repo.UpdateExecution(ctx, exec)

	return wf, nil
}

// recordStepExecution 记录步骤执行（Attempt=1，用于兼容旧调用）。
func (s *Service) recordStepExecution(ctx context.Context, workflowExecutionID int64, step *WorkflowStep, status StepStatus, message string, execErr error) {
	now := time.Now()
	startedAt := step.StartedAt
	if startedAt == nil {
		startedAt = &now
	}
	s.recordStepExecutionWithAttempt(ctx, workflowExecutionID, step, status, message, execErr, 1, startedAt, &now)
}

// recordStepExecutionWithAttempt 记录步骤执行（支持多次 attempt）。
// 每次 attempt 创建独立的 WorkflowStepExecution 记录。
func (s *Service) recordStepExecutionWithAttempt(ctx context.Context, workflowExecutionID int64, step *WorkflowStep, status StepStatus, message string, execErr error, attempt int, startedAt, finishedAt *time.Time) {
	stepExec := &WorkflowStepExecution{
		WorkflowExecutionID: workflowExecutionID,
		WorkflowStepID:      step.ID,
		StepName:            step.Name,
		StepType:            step.Type,
		ActionType:          step.ActionType,
		Status:              status,
		Attempt:             attempt,
		StartedAt:           startedAt,
		FinishedAt:          finishedAt,
		Result:              message,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
	if execErr != nil {
		stepExec.Error = execErr.Error()
	}
	if startedAt != nil && finishedAt != nil {
		stepExec.DurationMs = finishedAt.Sub(*startedAt).Milliseconds()
	}
	if err := s.repo.CreateStepExecution(ctx, stepExec); err != nil {
		slog.Error("记录步骤执行失败", "workflow_execution_id", workflowExecutionID, "step_id", step.ID, "error", err)
	}
}

// executeObservation 执行 observation 步骤：查询 Kubernetes Pod 状态（只读）。
func (s *Service) executeObservation(ctx context.Context, step *WorkflowStep, execContext map[string]interface{}) (bool, string, error) {
	if s.k8sQuerier == nil {
		return false, "Kubernetes 查询器未配置", fmt.Errorf("K8sQuerier not configured")
	}
	if step.TargetName == "" {
		return false, "observation 步骤缺少 target_name", fmt.Errorf("target_name is empty")
	}

	pod, err := s.k8sQuerier.GetPod(ctx, step.Cluster, step.Namespace, step.TargetName)
	if err != nil {
		return false, fmt.Sprintf("查询 Pod 失败: %v", err), err
	}

	// 构建观察结果
	result := PodObservationResult{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		UID:       string(pod.UID),
		Status:    string(pod.Status.Phase),
		NodeName:  pod.Spec.NodeName,
		PodIP:     pod.Status.PodIP,
	}
	if len(pod.Status.ContainerStatuses) > 0 {
		result.RestartCount = pod.Status.ContainerStatuses[0].RestartCount
	}

	// 保存 Pod UID 到执行上下文，供 verification 步骤使用
	execContext[fmt.Sprintf("pod_uid_%s_%s", step.Namespace, step.TargetName)] = string(pod.UID)

	resultJSON, _ := json.Marshal(result)
	message := fmt.Sprintf("Pod %s/%s 状态: %s, RestartCount: %d, UID: %s",
		step.Namespace, step.TargetName, result.Status, result.RestartCount, result.UID)
	return true, message + " | " + string(resultJSON), nil
}

// executeInvestigation 执行 investigation 步骤：查询 Pod 详细信息和事件（只读）。
func (s *Service) executeInvestigation(ctx context.Context, step *WorkflowStep) (bool, string, error) {
	if s.k8sQuerier == nil {
		return false, "Kubernetes 查询器未配置", fmt.Errorf("K8sQuerier not configured")
	}
	if step.TargetName == "" {
		return false, "investigation 步骤缺少 target_name", fmt.Errorf("target_name is empty")
	}

	pod, err := s.k8sQuerier.GetPod(ctx, step.Cluster, step.Namespace, step.TargetName)
	if err != nil {
		return false, fmt.Sprintf("查询 Pod 失败: %v", err), err
	}

	// 查询 Pod 事件
	events, err := s.k8sQuerier.GetPodEvents(ctx, step.Cluster, step.Namespace, step.TargetName)
	if err != nil {
		// 事件查询失败不影响调查结果
		events = nil
	}

	// 构建调查结果
	investigation := map[string]interface{}{
		"pod_name":      pod.Name,
		"namespace":     pod.Namespace,
		"uid":           string(pod.UID),
		"status":        string(pod.Status.Phase),
		"node_name":     pod.Spec.NodeName,
		"pod_ip":        pod.Status.PodIP,
		"conditions":    pod.Status.Conditions,
		"event_count":   len(events),
	}
	if len(pod.Status.ContainerStatuses) > 0 {
		cs := pod.Status.ContainerStatuses[0]
		investigation["restart_count"] = cs.RestartCount
		investigation["container_ready"] = cs.Ready
		investigation["container_state"] = cs.State
	}

	resultJSON, _ := json.Marshal(investigation)
	message := fmt.Sprintf("Pod %s/%s 调查完成: 状态=%s, 事件数=%d",
		step.Namespace, step.TargetName, investigation["status"], investigation["event_count"])
	return true, message + " | " + string(resultJSON), nil
}

// executeVerification 执行 verification 步骤：验证前序 automation 是否真正成功。
func (s *Service) executeVerification(ctx context.Context, step *WorkflowStep, execContext map[string]interface{}) (bool, string, error) {
	if s.k8sQuerier == nil {
		return false, "Kubernetes 查询器未配置", fmt.Errorf("K8sQuerier not configured")
	}
	if step.TargetName == "" {
		return false, "verification 步骤缺少 target_name", fmt.Errorf("target_name is empty")
	}

	// 从执行上下文获取执行前的 Pod UID
	oldUIDKey := fmt.Sprintf("pod_uid_%s_%s", step.Namespace, step.TargetName)
	oldUID, _ := execContext[oldUIDKey].(string)

	// 等待 Kubernetes 重建 Pod（最多等待 30 秒，每 5 秒重试一次）
	var pod *corev1.Pod
	var err error
	maxRetries := 6
	for i := 0; i < maxRetries; i++ {
		// 等待 5 秒
		select {
		case <-ctx.Done():
			return false, "验证超时: context cancelled", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		// 先尝试查询旧 Pod 名称
		pod, err = s.k8sQuerier.GetPod(ctx, step.Cluster, step.Namespace, step.TargetName)
		if err != nil {
			// 旧 Pod 不存在，可能已经被删除并重建（Deployment 管理的 Pod 名称会变化）
			// 通过 ListPods 查询所有 Pod，找到状态为 Running 的 Pod
			pods, listErr := s.k8sQuerier.ListPods(ctx, step.Cluster, step.Namespace)
			if listErr != nil {
				continue
			}
			// 找到状态为 Running 的 Pod（假设是新重建的 Pod）
			for _, p := range pods {
				if p.Status.Phase == "Running" {
					pod = &p
					err = nil
					break
				}
			}
			if pod == nil {
				continue
			}
		}

		currentUID := string(pod.UID)
		currentStatus := string(pod.Status.Phase)

		// 验证 Pod 状态为 Running
		if currentStatus != "Running" {
			continue
		}

		// 如果有旧 UID，验证 UID 是否变化（说明 Pod 已重启）
		if oldUID != "" && oldUID == currentUID {
			continue
		}

		// 验证通过
		verification := map[string]interface{}{
			"pod_name":      pod.Name,
			"namespace":     pod.Namespace,
			"old_uid":       oldUID,
			"current_uid":   currentUID,
			"uid_changed":   oldUID != "" && oldUID != currentUID,
			"status":        currentStatus,
			"verified":      true,
			"retry_count":   i,
			"pod_renamed":   pod.Name != step.TargetName,
		}
		if len(pod.Status.ContainerStatuses) > 0 {
			verification["restart_count"] = pod.Status.ContainerStatuses[0].RestartCount
		}

		resultJSON, _ := json.Marshal(verification)
		message := fmt.Sprintf("验证通过: Pod %s/%s 状态=%s, UID已变化=%v, 重试次数=%d, Pod重命名=%v",
			step.Namespace, pod.Name, currentStatus, verification["uid_changed"], i, verification["pod_renamed"])
		return true, message + " | " + string(resultJSON), nil
	}

	// 验证失败
	if err != nil {
		return false, fmt.Sprintf("验证失败: Pod 未在规定时间内恢复: %v", err),
			fmt.Errorf("pod not recovered within timeout: %v", err)
	}
	if pod != nil {
		currentUID := string(pod.UID)
		currentStatus := string(pod.Status.Phase)
		if currentStatus != "Running" {
			return false, fmt.Sprintf("验证失败: Pod 状态为 %s，期望 Running", currentStatus),
				fmt.Errorf("pod status is %s, expected Running", currentStatus)
		}
		if oldUID != "" && oldUID == currentUID {
			return false, fmt.Sprintf("验证失败: Pod UID 未变化 (%s)，重启可能未生效", currentUID),
				fmt.Errorf("pod UID not changed: %s", currentUID)
		}
	}
	return false, "验证失败: 未知错误", fmt.Errorf("unknown verification error")
}

// Cancel 取消工作流。
func (s *Service) Cancel(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if wf.Status == WorkflowStatusRunning {
		return nil, fmt.Errorf("运行中的工作流不能取消")
	}
	wf.Status = WorkflowStatusCancelled
	wf.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, wf); err != nil {
		return nil, err
	}
	return wf, nil
}

// GetExecutions 获取工作流的执行记录列表。
func (s *Service) GetExecutions(ctx context.Context, workflowID int64, page, pageSize int) ([]WorkflowExecution, int64, error) {
	return s.repo.ListExecutionsByWorkflowID(ctx, workflowID, page, pageSize)
}

// GetExecution 获取单个工作流执行记录。
func (s *Service) GetExecution(ctx context.Context, executionID int64) (*WorkflowExecution, error) {
	return s.repo.FindExecutionByID(ctx, executionID)
}
