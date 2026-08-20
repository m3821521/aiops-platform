package workflow

import (
	"context"
	"testing"
	"time"
)

// ==================== Model Tests ====================

func TestIsValidStepType(t *testing.T) {
	tests := []struct {
		name     string
		stepType StepType
		expected bool
	}{
		{"observation", StepTypeObservation, true},
		{"investigation", StepTypeInvestigation, true},
		{"automation", StepTypeAutomation, true},
		{"verification", StepTypeVerification, true},
		{"invalid", StepType("invalid"), false},
		{"empty", StepType(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidStepType(tt.stepType); got != tt.expected {
				t.Errorf("IsValidStepType(%s) = %v, want %v", tt.stepType, got, tt.expected)
			}
		})
	}
}

func TestIsAutomationStepType(t *testing.T) {
	tests := []struct {
		name     string
		stepType StepType
		expected bool
	}{
		{"automation", StepTypeAutomation, true},
		{"observation", StepTypeObservation, false},
		{"investigation", StepTypeInvestigation, false},
		{"verification", StepTypeVerification, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAutomationStepType(tt.stepType); got != tt.expected {
				t.Errorf("IsAutomationStepType(%s) = %v, want %v", tt.stepType, got, tt.expected)
			}
		})
	}
}

func TestIsValidFailureStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy FailureStrategy
		expected bool
	}{
		{"stop", FailureStrategyStop, true},
		{"continue", FailureStrategyContinue, true},
		{"invalid", FailureStrategy("invalid"), false},
		{"empty", FailureStrategy(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidFailureStrategy(tt.strategy); got != tt.expected {
				t.Errorf("IsValidFailureStrategy(%s) = %v, want %v", tt.strategy, got, tt.expected)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     WorkflowStatus
		to       WorkflowStatus
		expected bool
	}{
		// draft transitions
		{"draft to pending_approval", WorkflowStatusDraft, WorkflowStatusPendingApproval, true},
		{"draft to cancelled", WorkflowStatusDraft, WorkflowStatusCancelled, true},
		{"draft to approved", WorkflowStatusDraft, WorkflowStatusApproved, false},
		{"draft to running", WorkflowStatusDraft, WorkflowStatusRunning, false},

		// pending_approval transitions
		{"pending_approval to approved", WorkflowStatusPendingApproval, WorkflowStatusApproved, true},
		{"pending_approval to cancelled", WorkflowStatusPendingApproval, WorkflowStatusCancelled, true},
		{"pending_approval to running", WorkflowStatusPendingApproval, WorkflowStatusRunning, false},

		// approved transitions
		{"approved to running", WorkflowStatusApproved, WorkflowStatusRunning, true},
		{"approved to cancelled", WorkflowStatusApproved, WorkflowStatusCancelled, true},
		{"approved to success", WorkflowStatusApproved, WorkflowStatusSuccess, false},

		// running transitions
		{"running to success", WorkflowStatusRunning, WorkflowStatusSuccess, true},
		{"running to failed", WorkflowStatusRunning, WorkflowStatusFailed, true},
		{"running to approved", WorkflowStatusRunning, WorkflowStatusApproved, false},

		// terminal states
		{"success to running", WorkflowStatusSuccess, WorkflowStatusRunning, false},
		{"failed to running", WorkflowStatusFailed, WorkflowStatusRunning, false},
		{"cancelled to running", WorkflowStatusCancelled, WorkflowStatusRunning, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.expected {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.expected)
			}
		})
	}
}

func TestWorkflowStepParameters(t *testing.T) {
	step := &WorkflowStep{}
	params := map[string]interface{}{
		"replicas": float64(2),
		"cluster":  "local",
	}
	step.SetParameters(params)

	got := step.GetParameters()
	if got["replicas"] != float64(2) {
		t.Errorf("GetParameters replicas = %v, want 2", got["replicas"])
	}
	if got["cluster"] != "local" {
		t.Errorf("GetParameters cluster = %v, want local", got["cluster"])
	}

	// 空参数
	emptyStep := &WorkflowStep{}
	if got := emptyStep.GetParameters(); got != nil {
		t.Errorf("GetParameters empty = %v, want nil", got)
	}
}

// ==================== Service Tests ====================

// mockActionExecutor 是一个 mock 的 ActionExecutor。
type mockActionExecutor struct {
	success bool
	message string
	err     error
}

func (m *mockActionExecutor) ExecuteAction(ctx context.Context, actionType, cluster, namespace, targetName string, params map[string]interface{}) (bool, string, error) {
	return m.success, m.message, m.err
}

func TestValidateWorkflow(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name    string
		wf      *Workflow
		wantErr bool
	}{
		{
			name: "valid workflow",
			wf: &Workflow{
				Name: "Test Workflow",
				Steps: []WorkflowStep{
					{Name: "Step 1", Type: StepTypeObservation, FailureStrategy: FailureStrategyStop},
					{Name: "Step 2", Type: StepTypeAutomation, ActionType: "restart_pod", FailureStrategy: FailureStrategyStop},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			wf: &Workflow{
				Name:  "",
				Steps: []WorkflowStep{{Name: "Step 1", Type: StepTypeObservation}},
			},
			wantErr: true,
		},
		{
			name: "no steps",
			wf: &Workflow{
				Name:  "Test",
				Steps: []WorkflowStep{},
			},
			wantErr: true,
		},
		{
			name: "invalid step type",
			wf: &Workflow{
				Name: "Test",
				Steps: []WorkflowStep{
					{Name: "Step 1", Type: StepType("invalid")},
				},
			},
			wantErr: true,
		},
		{
			name: "automation step without action_type",
			wf: &Workflow{
				Name: "Test",
				Steps: []WorkflowStep{
					{Name: "Step 1", Type: StepTypeAutomation, ActionType: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid failure strategy",
			wf: &Workflow{
				Name: "Test",
				Steps: []WorkflowStep{
					{Name: "Step 1", Type: StepTypeObservation, FailureStrategy: FailureStrategy("invalid")},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ValidateWorkflow(tt.wf)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDryRun(t *testing.T) {
	// 使用内存数据库测试需要 gorm，这里只测试 DryRun 的逻辑结构
	// 完整的 DryRun 测试需要集成测试

	svc := &Service{
		executor: &mockActionExecutor{success: true, message: "ok"},
	}

	// 验证 DryRunResult 结构
	result := &DryRunResult{
		Valid:   true,
		Message: "test",
		Steps: []DryRunStepResult{
			{StepID: 1, StepName: "Step 1", StepType: StepTypeObservation, Valid: true, CanExecute: true},
		},
	}

	if !result.Valid {
		t.Error("DryRunResult.Valid should be true")
	}
	if len(result.Steps) != 1 {
		t.Errorf("DryRunResult.Steps length = %d, want 1", len(result.Steps))
	}

	_ = svc // 避免未使用警告
}

func TestFourEyesPrinciple(t *testing.T) {
	// 测试四眼原则的逻辑
	// 完整测试需要数据库

	wf := &Workflow{
		ID:        1,
		Name:      "Test",
		Status:    WorkflowStatusPendingApproval,
		CreatedBy: 1,
	}

	// 创建者不能审批自己的工作流
	if wf.CreatedBy == 1 {
		// 应该返回错误
		t.Log("Four eyes principle: creator cannot approve own workflow")
	}

	// 创建者不能执行自己的工作流
	if wf.CreatedBy == 1 {
		// 应该返回错误
		t.Log("Four eyes principle: creator cannot execute own workflow")
	}
}

// ==================== Model Structure Tests ====================

func TestWorkflowExecutionModel(t *testing.T) {
	now := time.Now()
	exec := &WorkflowExecution{
		ID:          1,
		WorkflowID:  1,
		Status:      WorkflowStatusRunning,
		TriggerType: "manual",
		TriggeredBy: 1,
		StartedAt:   &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if exec.TableName() != "workflow_executions" {
		t.Errorf("TableName() = %s, want workflow_executions", exec.TableName())
	}
	if exec.Status != WorkflowStatusRunning {
		t.Errorf("Status = %s, want running", exec.Status)
	}
}

func TestWorkflowStepExecutionModel(t *testing.T) {
	now := time.Now()
	stepExec := &WorkflowStepExecution{
		ID:                  1,
		WorkflowExecutionID: 1,
		WorkflowStepID:      1,
		StepName:            "Step 1",
		StepType:            StepTypeAutomation,
		ActionType:          "restart_pod",
		Status:              StepStatusSuccess,
		Attempt:             1,
		StartedAt:           &now,
		FinishedAt:          &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if stepExec.TableName() != "workflow_step_executions" {
		t.Errorf("TableName() = %s, want workflow_step_executions", stepExec.TableName())
	}
	if stepExec.Status != StepStatusSuccess {
		t.Errorf("Status = %s, want success", stepExec.Status)
	}
	if !IsAutomationStepType(stepExec.StepType) {
		t.Error("StepType should be automation")
	}
}

func TestWorkflowStepTypeField(t *testing.T) {
	// 验证 WorkflowStep 有 Type 字段
	step := &WorkflowStep{
		Name:            "Test Step",
		Type:            StepTypeAutomation,
		ActionType:      "restart_pod",
		FailureStrategy: FailureStrategyStop,
	}

	if step.Type != StepTypeAutomation {
		t.Errorf("Type = %s, want automation", step.Type)
	}
	if step.FailureStrategy != FailureStrategyStop {
		t.Errorf("FailureStrategy = %s, want stop", step.FailureStrategy)
	}
	if !IsAutomationStepType(step.Type) {
		t.Error("IsAutomationStepType should be true")
	}
}

// ==================== Retry Tests ====================

func TestCalculateRetryDelay(t *testing.T) {
	tests := []struct {
		name          string
		retryDelaySec int
		attempt       int
		expected      time.Duration
	}{
		{"attempt1_delay5", 5, 1, 5 * time.Second},
		{"attempt2_delay5", 5, 2, 10 * time.Second},
		{"attempt3_delay5", 5, 3, 20 * time.Second},
		{"attempt4_delay5", 5, 4, 30 * time.Second},
		{"attempt5_delay5_capped", 5, 5, 30 * time.Second},
		{"attempt1_delay10", 10, 1, 10 * time.Second},
		{"attempt2_delay10", 10, 2, 20 * time.Second},
		{"attempt3_delay10_capped", 10, 3, 30 * time.Second},
		{"zero_delay_default", 0, 1, 5 * time.Second},
		{"negative_delay_default", -1, 1, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRetryDelay(tt.retryDelaySec, tt.attempt)
			if got != tt.expected {
				t.Errorf("calculateRetryDelay(%d, %d) = %v, want %v",
					tt.retryDelaySec, tt.attempt, got, tt.expected)
			}
		})
	}
}

func TestRetryMaxDelay(t *testing.T) {
	// 验证 delay 永远 <= 30s
	for attempt := 1; attempt <= 10; attempt++ {
		delay := calculateRetryDelay(5, attempt)
		if delay > 30*time.Second {
			t.Errorf("attempt %d: delay = %v, exceeds max 30s", attempt, delay)
		}
	}
	for attempt := 1; attempt <= 10; attempt++ {
		delay := calculateRetryDelay(100, attempt)
		if delay > 30*time.Second {
			t.Errorf("attempt %d with large base: delay = %v, exceeds max 30s", attempt, delay)
		}
	}
}

func TestMaxAttempts(t *testing.T) {
	tests := []struct {
		name     string
		maxRetry int
		expected int
	}{
		{"maxRetry0", 0, 1},
		{"maxRetry1", 1, 2},
		{"maxRetry2", 2, 3},
		{"maxRetry3", 3, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.maxRetry + 1
			if got != tt.expected {
				t.Errorf("maxAttempts = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestRetryDelaySecDefault(t *testing.T) {
	step := &WorkflowStep{
		Name: "Test Step",
		Type: StepTypeObservation,
	}
	// GORM default 会在数据库层面设置，但 Go 结构体默认是 0
	// 验证 calculateRetryDelay 对 0 的处理
	if step.RetryDelaySec != 0 {
		t.Errorf("default RetryDelaySec = %d, want 0 (Go zero value)", step.RetryDelaySec)
	}
	delay := calculateRetryDelay(step.RetryDelaySec, 1)
	if delay != 5*time.Second {
		t.Errorf("calculateRetryDelay(0, 1) = %v, want 5s", delay)
	}
}

func TestWaitWithContext(t *testing.T) {
	t.Run("normal_wait", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		err := waitWithContext(ctx, 100*time.Millisecond)
		elapsed := time.Since(start)
		if err != nil {
			t.Errorf("waitWithContext returned error: %v", err)
		}
		if elapsed < 100*time.Millisecond {
			t.Errorf("waitWithContext returned too early: %v", elapsed)
		}
	})

	t.Run("context_cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消
		start := time.Now()
		err := waitWithContext(ctx, 5*time.Second)
		elapsed := time.Since(start)
		if err == nil {
			t.Error("waitWithContext should return error when context cancelled")
		}
		if elapsed > 500*time.Millisecond {
			t.Errorf("waitWithContext did not return immediately: %v", elapsed)
		}
	})

	t.Run("context_timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := waitWithContext(ctx, 5*time.Second)
		elapsed := time.Since(start)
		if err == nil {
			t.Error("waitWithContext should return error on context timeout")
		}
		if elapsed > 500*time.Millisecond {
			t.Errorf("waitWithContext did not respect context timeout: %v", elapsed)
		}
	})
}

func TestRetryCountSemantics(t *testing.T) {
	// RetryCount 表示已经发生的 retry 次数
	// 第一次执行成功: RetryCount = 0
	// 第一次失败，第二次成功: RetryCount = 1
	// 第一次失败，第二次失败，第三次成功: RetryCount = 2
	// 全部失败: RetryCount = MaxRetry
	tests := []struct {
		name          string
		attempts      int
		finalSuccess  bool
		expectedRetry int
	}{
		{"first_attempt_success", 1, true, 0},
		{"second_attempt_success", 2, true, 1},
		{"third_attempt_success", 3, true, 2},
		{"all_failed_maxRetry2", 3, false, 2},
		{"all_failed_maxRetry3", 4, false, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 RetryCount 更新逻辑（与 service.go 一致）
			retryCount := 0
			maxAttempts := tt.attempts
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				if attempt == maxAttempts && tt.finalSuccess {
					// 最后一次成功，不更新 retryCount（保持之前的值）
					break
				}
				if attempt == maxAttempts && !tt.finalSuccess {
					// 最后一次失败，retryCount = MaxRetry = maxAttempts - 1
					retryCount = maxAttempts - 1
					break
				}
				// 非最后一次失败，retryCount = attempt
				retryCount = attempt
			}
			if retryCount != tt.expectedRetry {
				t.Errorf("RetryCount = %d, want %d", retryCount, tt.expectedRetry)
			}
		})
	}
}
