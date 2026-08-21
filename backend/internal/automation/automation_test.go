package automation

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Action{}, &ActionExecution{}, &AutomationAudit{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	valid := []struct {
		from ActionStatus
		to   ActionStatus
	}{
		{StatusProposed, StatusPendingApproval},
		{StatusPendingApproval, StatusApproved},
		{StatusPendingApproval, StatusRejected},
		{StatusApproved, StatusRunning},
		{StatusRunning, StatusSuccess},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusTimeout},
		{StatusProposed, StatusCancelled},
		{StatusApproved, StatusCancelled},
	}
	for _, v := range valid {
		if !CanTransition(v.from, v.to) {
			t.Errorf("expected valid transition %s -> %s", v.from, v.to)
		}
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	invalid := []struct {
		from ActionStatus
		to   ActionStatus
	}{
		{StatusSuccess, StatusRunning},
		{StatusRejected, StatusApproved},
		{StatusFailed, StatusRunning},
		{StatusSuccess, StatusSuccess},
		{StatusPendingApproval, StatusRunning},
	}
	for _, v := range invalid {
		if CanTransition(v.from, v.to) {
			t.Errorf("expected invalid transition %s -> %s", v.from, v.to)
		}
	}
}

func TestDefaultRisk(t *testing.T) {
	if DefaultRisk(ActionRestartPod) != RiskMedium {
		t.Error("restart_pod should be medium")
	}
	if DefaultRisk(ActionScaleDeployment) != RiskHigh {
		t.Error("scale_deployment should be high")
	}
	if DefaultRisk(ActionJenkinsBuild) != RiskHigh {
		t.Error("jenkins_build should be high")
	}
	if DefaultRisk(ActionArgoCDSync) != RiskHigh {
		t.Error("argocd_sync should be high")
	}
}

func TestActionParameters(t *testing.T) {
	action := &Action{}
	action.SetParameters(map[string]interface{}{"replicas": 5})
	params := action.GetParameters()
	if params["replicas"].(float64) != 5 {
		t.Error("parameters round-trip failed")
	}
}

func TestActionRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	action := &Action{
		ActionType: ActionRestartPod,
		TargetType: "pod",
		TargetName: "test-pod",
		Namespace:  "default",
		Cluster:    "local",
		Status:     StatusPendingApproval,
		Risk:       RiskMedium,
		Reason:     "test",
	}
	if err := repo.Create(ctx, action); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if action.ID == 0 {
		t.Error("expected non-zero ID")
	}

	found, err := repo.FindByID(ctx, action.ID)
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if found.TargetName != "test-pod" {
		t.Errorf("expected test-pod, got %s", found.TargetName)
	}

	actions, total, err := repo.List(ctx, ListFilter{Status: StatusPendingApproval}, 1, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
}

func TestService_CreateAndApprove(t *testing.T) {
	db := setupTestDB(t)
	actionRepo := NewActionRepository(db)
	execRepo := NewExecutionRepository(db)
	auditRepo := NewAuditRepository(db)
	policy := NewPolicyEngine("development")
	svc := NewService(actionRepo, execRepo, auditRepo, policy, nil, nil, nil)
	ctx := context.Background()

	action := &Action{
		ActionType: ActionRestartPod,
		TargetType: "pod",
		TargetName: "test-pod",
		Namespace:  "default",
		Cluster:    "local",
		Reason:     "OOMKilled",
	}
	created, err := svc.CreateAction(ctx, action, 1)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if created.Status != StatusPendingApproval {
		t.Errorf("expected pending_approval, got %s", created.Status)
	}
	if created.Risk != RiskMedium {
		t.Errorf("expected medium risk, got %s", created.Risk)
	}

	approved, err := svc.Approve(ctx, created.ID, 2)
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if approved.Status != StatusApproved {
		t.Errorf("expected approved, got %s", approved.Status)
	}
	if approved.ApprovedBy != 2 {
		t.Errorf("expected approved_by=2, got %d", approved.ApprovedBy)
	}
}

func TestService_Reject(t *testing.T) {
	db := setupTestDB(t)
	actionRepo := NewActionRepository(db)
	execRepo := NewExecutionRepository(db)
	auditRepo := NewAuditRepository(db)
	policy := NewPolicyEngine("development")
	svc := NewService(actionRepo, execRepo, auditRepo, policy, nil, nil, nil)
	ctx := context.Background()

	action := &Action{
		ActionType: ActionRestartPod,
		TargetType: "pod",
		TargetName: "test-pod",
		Namespace:  "default",
		Cluster:    "local",
	}
	created, _ := svc.CreateAction(ctx, action, 1)

	rejected, err := svc.Reject(ctx, created.ID, 2, "not needed")
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if rejected.Status != StatusRejected {
		t.Errorf("expected rejected, got %s", rejected.Status)
	}
	if rejected.RejectReason != "not needed" {
		t.Errorf("expected reason 'not needed', got %s", rejected.RejectReason)
	}
}

func TestService_ExecuteWithoutApproval(t *testing.T) {
	db := setupTestDB(t)
	actionRepo := NewActionRepository(db)
	execRepo := NewExecutionRepository(db)
	auditRepo := NewAuditRepository(db)
	policy := NewPolicyEngine("development")
	svc := NewService(actionRepo, execRepo, auditRepo, policy, nil, nil, nil)
	ctx := context.Background()

	action := &Action{
		ActionType: ActionRestartPod,
		TargetType: "pod",
		TargetName: "test-pod",
		Namespace:  "default",
		Cluster:    "local",
	}
	created, _ := svc.CreateAction(ctx, action, 1)

	_, err := svc.Execute(ctx, created.ID, 1)
	if err == nil {
		t.Error("expected error when executing without approval")
	}
}

func TestService_Cancel(t *testing.T) {
	db := setupTestDB(t)
	actionRepo := NewActionRepository(db)
	execRepo := NewExecutionRepository(db)
	auditRepo := NewAuditRepository(db)
	policy := NewPolicyEngine("development")
	svc := NewService(actionRepo, execRepo, auditRepo, policy, nil, nil, nil)
	ctx := context.Background()

	action := &Action{
		ActionType: ActionRestartPod,
		TargetType: "pod",
		TargetName: "test-pod",
		Namespace:  "default",
		Cluster:    "local",
	}
	created, _ := svc.CreateAction(ctx, action, 1)

	cancelled, err := svc.Cancel(ctx, created.ID, 1)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Errorf("expected cancelled, got %s", cancelled.Status)
	}
}

func TestPolicyEngine_Check(t *testing.T) {
	policy := NewPolicyEngine("development")
	action := Action{ActionType: ActionRestartPod, Risk: RiskMedium}
	check := policy.Check(context.Background(), action)
	if !check.Allowed {
		t.Error("expected allowed")
	}
	if !check.RequireApproval {
		t.Error("expected require approval")
	}
}

func TestPolicyEngine_StateTransition(t *testing.T) {
	policy := NewPolicyEngine("development")
	if err := policy.ValidateStateTransition(StatusPendingApproval, StatusApproved); err != nil {
		t.Errorf("expected valid: %v", err)
	}
	if err := policy.ValidateStateTransition(StatusSuccess, StatusRunning); err == nil {
		t.Error("expected invalid transition")
	}
}

func TestRedactSensitive(t *testing.T) {
	input := `{"password":"secret123","token":"abc","normal":"value"}`
	result := redactSensitive(input)
	if result == input {
		t.Error("expected redaction")
	}
	if contains(result, "secret123") {
		t.Error("password should be redacted")
	}
	if !contains(result, "[REDACTED]") {
		t.Error("expected [REDACTED] marker")
	}
}

func TestAuditRepository(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditRepository(db)
	ctx := context.Background()

	audit := &AutomationAudit{
		ActionID:   1,
		UserID:     1,
		Operation:  "create",
		Target:     "pod/test",
		RequestJSON: `{"password":"secret"}`,
		Status:     "pending_approval",
		CreatedAt:  time.Now(),
	}
	if err := repo.Create(ctx, audit); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if contains(audit.RequestJSON, "secret") {
		t.Error("sensitive data should be redacted in audit")
	}

	audits, total, err := repo.List(ctx, 0, 0, 0, 1, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
	if len(audits) != 1 {
		t.Errorf("expected 1, got %d", len(audits))
	}
}

func TestExecutionRepository(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	exec := &ActionExecution{
		ActionID: 1,
		Executor: "kubernetes",
		Status:   "success",
	}
	if err := repo.Create(ctx, exec); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	execs, err := repo.ListByActionID(ctx, 1)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(execs) != 1 {
		t.Errorf("expected 1, got %d", len(execs))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

// TestIsSupportedActionType 测试 Automation 支持的操作类型白名单。
// 只允许 restart_pod, scale_deployment, jenkins_build, argocd_sync。
// 其他类型 (observe, investigate, restart, scale, rollback, config_change, network_check)
// 不属于 Automation，应该走 Investigation / Monitoring 流程。
func TestIsSupportedActionType(t *testing.T) {
	// 支持的类型
	supported := []string{
		ActionRestartPod,      // restart_pod
		ActionScaleDeployment, // scale_deployment
		ActionJenkinsBuild,    // jenkins_build
		ActionArgoCDSync,      // argocd_sync
	}
	for _, actionType := range supported {
		if !IsSupportedActionType(actionType) {
			t.Errorf("expected %s to be supported", actionType)
		}
	}

	// 不支持的类型 (AI Recommendation 的 action_type)
	unsupported := []string{
		"observe",
		"investigate",
		"restart",
		"scale",
		"rollback",
		"config_change",
		"network_check",
		"",
		"invalid",
	}
	for _, actionType := range unsupported {
		if IsSupportedActionType(actionType) {
			t.Errorf("expected %s to be unsupported", actionType)
		}
	}
}

// TestGetSupportedActionTypes 测试获取支持的操作类型列表。
func TestGetSupportedActionTypes(t *testing.T) {
	types := GetSupportedActionTypes()
	if len(types) != 4 {
		t.Errorf("expected 4 supported types, got %d", len(types))
	}
	// 验证包含所有 4 种类型
	typeMap := make(map[string]bool)
	for _, t := range types {
		typeMap[t] = true
	}
	expected := []string{ActionRestartPod, ActionScaleDeployment, ActionJenkinsBuild, ActionArgoCDSync}
	for _, e := range expected {
		if !typeMap[e] {
			t.Errorf("expected %s in supported types", e)
		}
	}
}

// TestCreateAction_InvalidActionType 测试创建非法 action_type 的 Action。
// 应该返回错误，不允许写入数据库。
func TestCreateAction_InvalidActionType(t *testing.T) {
	db := setupTestDB(t)
	actionRepo := NewActionRepository(db)
	execRepo := NewExecutionRepository(db)
	auditRepo := NewAuditRepository(db)
	policy := NewPolicyEngine("development")

	service := NewService(actionRepo, execRepo, auditRepo, policy, nil, nil, nil)

	// 测试 investigate 类型
	investigateAction := &Action{
		ActionType: "investigate",
		TargetName: "test-pod",
		Cluster:    "local",
		Namespace:  "default",
	}
	_, err := service.CreateAction(context.Background(), investigateAction, 1)
	if err == nil {
		t.Error("expected error for investigate action_type, got nil")
	}
	if !contains(err.Error(), "不支持的操作类型") {
		t.Errorf("expected error to contain '不支持的操作类型', got: %v", err)
	}

	// 测试 restart 类型 (短名，应该映射为 restart_pod 才能使用)
	restartAction := &Action{
		ActionType: "restart",
		TargetName: "test-pod",
		Cluster:    "local",
		Namespace:  "default",
	}
	_, err = service.CreateAction(context.Background(), restartAction, 1)
	if err == nil {
		t.Error("expected error for restart action_type, got nil")
	}

	// 测试 scale 类型 (短名，应该映射为 scale_deployment 才能使用)
	scaleAction := &Action{
		ActionType: "scale",
		TargetName: "test-deployment",
		Cluster:    "local",
		Namespace:  "default",
	}
	_, err = service.CreateAction(context.Background(), scaleAction, 1)
	if err == nil {
		t.Error("expected error for scale action_type, got nil")
	}

	// 验证数据库中没有创建任何 Action
	actions, _, err := actionRepo.List(context.Background(), ListFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions in database, got %d", len(actions))
	}
}

// TestCreateAction_ValidActionType 测试创建合法 action_type 的 Action。
func TestCreateAction_ValidActionType(t *testing.T) {
	db := setupTestDB(t)
	actionRepo := NewActionRepository(db)
	execRepo := NewExecutionRepository(db)
	auditRepo := NewAuditRepository(db)
	policy := NewPolicyEngine("development")

	service := NewService(actionRepo, execRepo, auditRepo, policy, nil, nil, nil)

	// 测试 restart_pod 类型
	validAction := &Action{
		ActionType: ActionRestartPod,
		TargetName: "test-pod",
		Cluster:    "local",
		Namespace:  "default",
	}
	result, err := service.CreateAction(context.Background(), validAction, 1)
	if err != nil {
		t.Fatalf("expected success for restart_pod, got error: %v", err)
	}
	if result.ActionType != ActionRestartPod {
		t.Errorf("expected action_type %s, got %s", ActionRestartPod, result.ActionType)
	}
	if result.Status != StatusPendingApproval {
		t.Errorf("expected status %s, got %s", StatusPendingApproval, result.Status)
	}

	// 验证数据库中创建了 Action
	actions, _, err := actionRepo.List(context.Background(), ListFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(actions) != 1 {
		t.Errorf("expected 1 action in database, got %d", len(actions))
	}
}

// TestRecoverStaleActions 验证服务启动时恢复因 Worker 崩溃而永久停留在 RUNNING 状态的 Action。
func TestRecoverStaleActions(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	svc := NewService(repo, NewExecutionRepository(db), NewAuditRepository(db), NewPolicyEngine("test"), nil, nil, nil)

	ctx := context.Background()
	now := time.Now()

	// P2-02: Startup Recovery 不再使用 updated_at threshold。
	// 同步执行模型下，服务重启意味着所有 running 都已中断，统一恢复。
	// 创建一个 stale RUNNING Action（UpdatedAt 在 10 分钟前）
	staleAction := &Action{
		ActionType: ActionRestartPod,
		Status:     StatusRunning,
		TargetType: "pod",
		TargetName: "test-pod",
		Cluster:    "test",
		Namespace:  "default",
		CreatedAt:  now.Add(-10 * time.Minute),
		UpdatedAt:  now.Add(-10 * time.Minute),
	}
	if err := repo.Create(ctx, staleAction); err != nil {
		t.Fatalf("create stale action failed: %v", err)
	}

	// 创建一个新鲜的 RUNNING Action（UpdatedAt 在 1 分钟前）
	// P2-02: 启动时也会被恢复（同步执行模型下重启意味着中断）
	freshAction := &Action{
		ActionType: ActionRestartPod,
		Status:     StatusRunning,
		TargetType: "pod",
		TargetName: "test-pod-2",
		Cluster:    "test",
		Namespace:  "default",
		CreatedAt:  now.Add(-1 * time.Minute),
		UpdatedAt:  now.Add(-1 * time.Minute),
	}
	if err := repo.Create(ctx, freshAction); err != nil {
		t.Fatalf("create fresh action failed: %v", err)
	}

	// 执行启动恢复（threshold 参数保留用于兼容，但不再使用）
	recovered, err := svc.RecoverStaleActions(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("recover stale actions failed: %v", err)
	}
	// P2-02: 启动时所有 running 都被恢复
	if recovered != 2 {
		t.Errorf("expected 2 recovered actions on startup, got %d", recovered)
	}

	// 验证 stale Action 已被标记为 TIMEOUT
	recoveredStale, err := repo.FindByID(ctx, staleAction.ID)
	if err != nil {
		t.Fatalf("find stale action failed: %v", err)
	}
	if recoveredStale.Status != StatusTimeout {
		t.Errorf("expected stale action status %s, got %s", StatusTimeout, recoveredStale.Status)
	}

	// P2-02: 验证 fresh Action 也被标记为 TIMEOUT（启动时全量恢复）
	recoveredFresh, err := repo.FindByID(ctx, freshAction.ID)
	if err != nil {
		t.Fatalf("find fresh action failed: %v", err)
	}
	if recoveredFresh.Status != StatusTimeout {
		t.Errorf("expected fresh action status %s on startup recovery, got %s", StatusTimeout, recoveredFresh.Status)
	}
}

// TestRecoverStaleActions_NoStale 验证没有 stale Action 时恢复操作不影响现有数据。
func TestRecoverStaleActions_NoStale(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	svc := NewService(repo, NewExecutionRepository(db), NewAuditRepository(db), NewPolicyEngine("test"), nil, nil, nil)

	ctx := context.Background()

	// 创建一个 approved 状态的 Action（不是 RUNNING，不应被恢复）
	approvedAction := &Action{
		ActionType: ActionRestartPod,
		Status:     StatusApproved,
		TargetType: "pod",
		TargetName: "test-pod",
		Cluster:    "test",
		Namespace:  "default",
	}
	if err := repo.Create(ctx, approvedAction); err != nil {
		t.Fatalf("create approved action failed: %v", err)
	}

	// 执行恢复
	recovered, err := svc.RecoverStaleActions(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("recover stale actions failed: %v", err)
	}
	if recovered != 0 {
		t.Errorf("expected 0 recovered actions, got %d", recovered)
	}

	// 验证 approved Action 状态不变
	action, err := repo.FindByID(ctx, approvedAction.ID)
	if err != nil {
		t.Fatalf("find action failed: %v", err)
	}
	if action.Status != StatusApproved {
		t.Errorf("expected action status %s, got %s", StatusApproved, action.Status)
	}
}

// ==================== P2-02 Lease / Heartbeat / Recovery Tests ====================

func createRunningAction(t *testing.T, repo *ActionRepository, leaseExpiresAt *time.Time) *Action {
	t.Helper()
	action := &Action{
		ActionType:     ActionRestartPod,
		Status:         StatusRunning,
		TargetType:     "pod",
		TargetName:     "test-pod",
		Cluster:        "test",
		Namespace:      "default",
		LeaseExpiresAt: leaseExpiresAt,
	}
	if err := repo.Create(context.Background(), action); err != nil {
		t.Fatalf("create running action failed: %v", err)
	}
	return action
}

func TestFreshRunningNotRecovered(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	// lease 未过期（未来时间）
	future := time.Now().Add(60 * time.Second)
	createRunningAction(t, repo, &future)

	count, err := repo.RecoverExpiredLease(ctx)
	if err != nil {
		t.Fatalf("recover expired lease failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 recovered, got %d", count)
	}
}

func TestExpiredLeaseRecovered(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	// lease 已过期（过去时间）
	past := time.Now().Add(-60 * time.Second)
	action := createRunningAction(t, repo, &past)

	count, err := repo.RecoverExpiredLease(ctx)
	if err != nil {
		t.Fatalf("recover expired lease failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 recovered, got %d", count)
	}

	// 验证状态
	updated, err := repo.FindByID(ctx, action.ID)
	if err != nil {
		t.Fatalf("find action failed: %v", err)
	}
	if updated.Status != StatusTimeout {
		t.Errorf("expected status %s, got %s", StatusTimeout, updated.Status)
	}
}

func TestLeaseRefresh(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	past := time.Now().Add(-60 * time.Second)
	action := createRunningAction(t, repo, &past)

	// 刷新 lease
	if err := repo.RefreshLease(ctx, action.ID, 60*time.Second); err != nil {
		t.Fatalf("refresh lease failed: %v", err)
	}

	// 验证 lease 已更新（不应被恢复）
	updated, err := repo.FindByID(ctx, action.ID)
	if err != nil {
		t.Fatalf("find action failed: %v", err)
	}
	if updated.LeaseExpiresAt == nil || updated.LeaseExpiresAt.Before(time.Now()) {
		t.Errorf("expected lease expires in future, got %v", updated.LeaseExpiresAt)
	}

	count, err := repo.RecoverExpiredLease(ctx)
	if err != nil {
		t.Fatalf("recover expired lease failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 recovered after refresh, got %d", count)
	}
}

func TestRecoveryNullLeaseNotRecovered(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	// lease 为 NULL（旧数据）
	createRunningAction(t, repo, nil)

	count, err := repo.RecoverExpiredLease(ctx)
	if err != nil {
		t.Fatalf("recover expired lease failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 recovered for null lease, got %d", count)
	}
}

func TestUpdateStatusIfRunning_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	future := time.Now().Add(60 * time.Second)
	action := createRunningAction(t, repo, &future)

	updated, err := repo.UpdateStatusIfRunning(ctx, action.ID, StatusSuccess)
	if err != nil {
		t.Fatalf("update status if running failed: %v", err)
	}
	if !updated {
		t.Error("expected update success")
	}

	result, _ := repo.FindByID(ctx, action.ID)
	if result.Status != StatusSuccess {
		t.Errorf("expected status %s, got %s", StatusSuccess, result.Status)
	}
	if result.LeaseExpiresAt != nil {
		t.Error("expected lease cleared after final status update")
	}
}

func TestUpdateStatusIfRunning_AlreadyChanged(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	future := time.Now().Add(60 * time.Second)
	action := createRunningAction(t, repo, &future)

	// 先更新为 failed
	action.Status = StatusFailed
	repo.Update(ctx, action)

	// 再尝试 CAS 更新为 success（应该失败）
	updated, err := repo.UpdateStatusIfRunning(ctx, action.ID, StatusSuccess)
	if err != nil {
		t.Fatalf("update status if running failed: %v", err)
	}
	if updated {
		t.Error("expected update failure because status already changed")
	}

	result, _ := repo.FindByID(ctx, action.ID)
	if result.Status != StatusFailed {
		t.Errorf("expected status %s preserved, got %s", StatusFailed, result.Status)
	}
}

func TestActionRecovery_SetsTimeout(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	past := time.Now().Add(-60 * time.Second)
	action := createRunningAction(t, repo, &past)

	count, err := repo.RecoverExpiredLease(ctx)
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 recovered, got %d", count)
	}

	result, err := repo.FindByID(ctx, action.ID)
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if result.Status != StatusTimeout {
		t.Errorf("expected status %s, got %s", StatusTimeout, result.Status)
	}
	if result.FinishedAt == nil {
		t.Error("expected finished_at set")
	}
	if result.RejectReason == "" {
		t.Error("expected reject_reason set")
	}
	if result.LeaseExpiresAt != nil {
		t.Error("expected lease cleared")
	}
}

func TestStartupRecovery_ExistingRunning(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	// 创建多个 running action（包括有 lease 和无 lease 的）
	createRunningAction(t, repo, nil)
	future := time.Now().Add(60 * time.Second)
	createRunningAction(t, repo, &future)

	// 启动时恢复（不再使用 threshold，直接标记所有 running）
	count, err := repo.MarkStaleRunningAsTimeout(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("startup recovery failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 recovered on startup, got %d", count)
	}
}

func TestRecoveryCAS_Race(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActionRepository(db)
	ctx := context.Background()

	past := time.Now().Add(-60 * time.Second)
	createRunningAction(t, repo, &past)

	// 模拟两个实例同时恢复
	done := make(chan int64, 2)
	go func() {
		c, _ := repo.RecoverExpiredLease(ctx)
		done <- c
	}()
	go func() {
		c, _ := repo.RecoverExpiredLease(ctx)
		done <- c
	}()

	c1 := <-done
	c2 := <-done
	total := c1 + c2
	if total != 1 {
		t.Errorf("expected total 1 recovered (CAS race), got %d (%d + %d)", total, c1, c2)
	}
}
