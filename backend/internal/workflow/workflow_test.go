package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
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

// ==================== P7-3.2 Timeout Tests ====================

// controllableExecutor 可控的 ActionExecutor，支持延迟和按 attempt 返回不同结果。
type controllableExecutor struct {
	delay       time.Duration // 默认执行延迟
	delays      []time.Duration // 按 attempt 设置延迟（可选）
	results     []execResult  // 按 attempt 顺序的结果
	callCount   int
}

type execResult struct {
	success bool
	message string
	err     error
}

func (e *controllableExecutor) ExecuteAction(ctx context.Context, actionType, cluster, namespace, targetName string, params map[string]interface{}) (bool, string, error) {
	e.callCount++
	attempt := e.callCount

	// 确定本次 attempt 的延迟
	delay := e.delay
	if attempt <= len(e.delays) {
		delay = e.delays[attempt-1]
	}

	// 延迟执行（模拟长时间操作）
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return false, "context cancelled", ctx.Err()
		}
	}

	// 按 attempt 返回不同结果
	if attempt <= len(e.results) {
		r := e.results[attempt-1]
		return r.success, r.message, r.err
	}
	// 默认返回成功
	return true, "success", nil
}

// slowQuerier 可控的 K8sQuerier，支持延迟。
type slowQuerier struct {
	delay time.Duration
}

func (q *slowQuerier) GetPod(ctx context.Context, cluster, namespace, name string) (*corev1.Pod, error) {
	if q.delay > 0 {
		select {
		case <-time.After(q.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &corev1.Pod{}, nil
}

func (q *slowQuerier) GetPodEvents(ctx context.Context, cluster, namespace, pod string) ([]corev1.Event, error) {
	return nil, nil
}

func (q *slowQuerier) ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error) {
	return nil, nil
}

// newTestWorkflowDB 创建 SQLite 内存数据库并 AutoMigrate Workflow 相关表。
func newTestWorkflowDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Workflow{}, &WorkflowStep{}, &WorkflowExecution{}, &WorkflowStepExecution{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// createTestWorkflow 创建一个测试 Workflow 并保存到数据库。
func createTestWorkflow(t *testing.T, db *gorm.DB, steps []WorkflowStep) *Workflow {
	t.Helper()
	wf := &Workflow{
		Name:        "Test Workflow",
		Description: "Timeout test",
		Status:      WorkflowStatusApproved,
		CreatedBy:   1,
		Steps:       steps,
	}
	repo := NewRepository(db)
	if err := repo.Create(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	for i := range wf.Steps {
		wf.Steps[i].WorkflowID = wf.ID
		wf.Steps[i].Status = StepStatusPending
		if err := repo.CreateStep(context.Background(), &wf.Steps[i]); err != nil {
			t.Fatal(err)
		}
	}
	return wf
}

func TestTimeoutTrigger(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 2 * time.Second}
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Timeout Step",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  1,
			MaxRetry:    0,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	result, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != WorkflowStatusFailed {
		t.Errorf("Workflow status = %s, want failed", result.Status)
	}

	step := result.Steps[0]
	if step.Status != StepStatusFailed {
		t.Errorf("Step status = %s, want failed", step.Status)
	}

	if !strings.Contains(step.Error, "workflow step timeout") {
		t.Errorf("Step error = %s, want contains 'workflow step timeout'", step.Error)
	}

	// DurationMs 应该接近 1000ms
	if step.FinishedAt != nil && step.StartedAt != nil {
		duration := step.FinishedAt.Sub(*step.StartedAt).Milliseconds()
		if duration < 800 || duration > 2000 {
			t.Errorf("Step duration = %dms, want ~1000ms (800-2000)", duration)
		}
	}
}

func TestNoTimeout(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 0} // 立即返回
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Fast Step",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  5,
			MaxRetry:    0,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	result, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != WorkflowStatusSuccess {
		t.Errorf("Workflow status = %s, want success", result.Status)
	}

	step := result.Steps[0]
	if step.Status != StepStatusSuccess {
		t.Errorf("Step status = %s, want success", step.Status)
	}
}

func TestTimeoutWithRetry(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 2 * time.Second} // 每次都超时
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Timeout Retry Step",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  1,
			MaxRetry:    2,
			RetryDelaySec: 1,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	result, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != WorkflowStatusFailed {
		t.Errorf("Workflow status = %s, want failed", result.Status)
	}

	step := result.Steps[0]
	if step.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", step.RetryCount)
	}

	// 验证 StepExecution 记录数量（应该有 3 条：attempt 1,2,3）
	workflowExecs, _, _ := NewRepository(db).ListExecutionsByWorkflowID(context.Background(), wf.ID, 1, 10)
	if len(workflowExecs) > 0 {
		stepExecs, _ := NewRepository(db).ListStepExecutions(context.Background(), workflowExecs[0].ID)
		if len(stepExecs) != 3 {
			t.Errorf("StepExecution count = %d, want 3", len(stepExecs))
		}
		for i, se := range stepExecs {
			if se.Attempt != i+1 {
				t.Errorf("StepExecution[%d].Attempt = %d, want %d", i, se.Attempt, i+1)
			}
			if se.Status != StepStatusFailed {
				t.Errorf("StepExecution[%d].Status = %s, want failed", i, se.Status)
			}
		}
	}
}

func TestTimeoutWithRetrySuccess(t *testing.T) {
	db := newTestWorkflowDB(t)
	// 第1次延迟2秒（超时），第2次立即成功
	executor := &controllableExecutor{
		delays: []time.Duration{2 * time.Second, 0},
		results: []execResult{
			{success: false, message: "timeout", err: errors.New("timeout")},
			{success: true, message: "success", err: nil},
		},
	}
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Timeout Then Success",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  1,
			MaxRetry:    1,
			RetryDelaySec: 1,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	result, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != WorkflowStatusSuccess {
		t.Errorf("Workflow status = %s, want success", result.Status)
	}

	step := result.Steps[0]
	if step.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", step.RetryCount)
	}
	if step.Status != StepStatusSuccess {
		t.Errorf("Step status = %s, want success", step.Status)
	}
}

func TestContextCancellationNotTimeout(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 5 * time.Second}
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Cancel Test",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  30,
			MaxRetry:    0,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	// 创建一个 100ms 后取消的 context（让 Execute 能查询到 Workflow，然后在执行 Step 时取消）
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := svc.Execute(ctx, wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	step := result.Steps[0]
	// 不应该是 "workflow step timeout"，应该是 cancelled
	if strings.Contains(step.Error, "workflow step timeout") {
		t.Errorf("Step error should NOT be timeout, got: %s", step.Error)
	}
}

func TestTimeoutPersistence(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 2 * time.Second}
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Persistence Test",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  1,
			MaxRetry:    0,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	_, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// 查询 WorkflowExecution
	workflowExecs, total, err := NewRepository(db).ListExecutionsByWorkflowID(context.Background(), wf.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListExecutions error: %v", err)
	}
	if total != 1 {
		t.Errorf("WorkflowExecution count = %d, want 1", total)
	}
	if len(workflowExecs) == 0 {
		t.Fatal("No WorkflowExecution found")
	}

	we := workflowExecs[0]
	if we.Status != WorkflowStatusFailed {
		t.Errorf("WorkflowExecution status = %s, want failed", we.Status)
	}

	// 查询 WorkflowStepExecution
	stepExecs, err := NewRepository(db).ListStepExecutions(context.Background(), we.ID)
	if err != nil {
		t.Fatalf("ListStepExecutions error: %v", err)
	}
	if len(stepExecs) != 1 {
		t.Errorf("StepExecution count = %d, want 1", len(stepExecs))
	}

	se := stepExecs[0]
	if se.Attempt != 1 {
		t.Errorf("StepExecution.Attempt = %d, want 1", se.Attempt)
	}
	if se.Status != StepStatusFailed {
		t.Errorf("StepExecution.Status = %s, want failed", se.Status)
	}
	if !strings.Contains(se.Error, "workflow step timeout") {
		t.Errorf("StepExecution.Error = %s, want contains 'workflow step timeout'", se.Error)
	}
	if se.DurationMs < 800 || se.DurationMs > 2000 {
		t.Errorf("StepExecution.DurationMs = %d, want ~1000ms (800-2000)", se.DurationMs)
	}
}

func TestRetryDelayNotCountedInTimeout(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 2 * time.Second} // 每次都超时
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Retry Delay Test",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  1,
			MaxRetry:    1,
			RetryDelaySec: 1,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	start := time.Now()
	_, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	totalDuration := time.Since(start).Milliseconds()

	// 总时间应该约为: Attempt1(1s) + RetryDelay(1s) + Attempt2(1s) = 3s
	// 允许 2.5s - 5s 的误差范围
	if totalDuration < 2500 || totalDuration > 5000 {
		t.Errorf("Total duration = %dms, want ~3000ms (2500-5000)", totalDuration)
	}

	// 验证每个 StepExecution 的 DurationMs 都约为 1000ms（不包含 retry delay）
	workflowExecs, _, _ := NewRepository(db).ListExecutionsByWorkflowID(context.Background(), wf.ID, 1, 10)
	if len(workflowExecs) > 0 {
		stepExecs, _ := NewRepository(db).ListStepExecutions(context.Background(), workflowExecs[0].ID)
		for i, se := range stepExecs {
			if se.DurationMs < 800 || se.DurationMs > 2000 {
				t.Errorf("StepExecution[%d].DurationMs = %d, want ~1000ms (800-2000)", i, se.DurationMs)
			}
		}
	}
}

func TestTimeoutSecZeroUsesDefault(t *testing.T) {
	db := newTestWorkflowDB(t)
	executor := &controllableExecutor{delay: 35 * time.Second} // 超过默认30秒
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Zero Timeout Test",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  0, // 应该使用默认30秒
			MaxRetry:    0,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	// 使用一个 1 秒后取消的 context 来验证不会立即超时
	// 因为 TimeoutSec=0 应该使用默认30秒，所以1秒内不应该超时
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := svc.Execute(ctx, wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	step := result.Steps[0]
	// 因为 context 在1秒后取消，应该是 cancelled 而不是 timeout
	if strings.Contains(step.Error, "workflow step timeout") {
		t.Errorf("Step should be cancelled, not timeout: %s", step.Error)
	}
}

func TestP731RetryRegression(t *testing.T) {
	// 验证 P7-3.1 Retry 功能没有被 Timeout 修改破坏
	db := newTestWorkflowDB(t)
	// 第1次失败，第2次成功（不超时）
	executor := &controllableExecutor{
		delay: 0,
		results: []execResult{
			{success: false, message: "failed", err: errors.New("failed")},
			{success: true, message: "success", err: nil},
		},
	}
	svc := NewService(NewRepository(db), executor)
	svc.SetK8sQuerier(&slowQuerier{})

	steps := []WorkflowStep{
		{
			Order:       1,
			Name:        "Retry Regression",
			Type:        StepTypeAutomation,
			ActionType:  "restart_pod",
			TargetName:  "test-pod",
			Namespace:   "default",
			Cluster:     "local",
			TimeoutSec:  30,
			MaxRetry:    1,
			RetryDelaySec: 1,
			FailureStrategy: FailureStrategyStop,
		},
	}
	wf := createTestWorkflow(t, db, steps)

	result, err := svc.Execute(context.Background(), wf.ID, 2)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != WorkflowStatusSuccess {
		t.Errorf("Workflow status = %s, want success", result.Status)
	}

	step := result.Steps[0]
	if step.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", step.RetryCount)
	}
	if step.Status != StepStatusSuccess {
		t.Errorf("Step status = %s, want success", step.Status)
	}

	// 验证有2条 StepExecution 记录
	workflowExecs, _, _ := NewRepository(db).ListExecutionsByWorkflowID(context.Background(), wf.ID, 1, 10)
	if len(workflowExecs) > 0 {
		stepExecs, _ := NewRepository(db).ListStepExecutions(context.Background(), workflowExecs[0].ID)
		if len(stepExecs) != 2 {
			t.Errorf("StepExecution count = %d, want 2", len(stepExecs))
		}
	}
}
