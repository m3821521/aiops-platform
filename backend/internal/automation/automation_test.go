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
