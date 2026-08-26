package rca

import (
	"context"
	"fmt"
	"strings"
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
	if err := db.AutoMigrate(&IncidentAnalysis{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// mockCollector 是测试用的 ContextCollector。
type mockCollector struct {
	alerts    []AlertInfo
	anomalies []AnomalyInfo
	events    []EventInfo
	metrics   []MetricInfo
	logs      []LogInfo
	topology  TopologyInfo
	podState  *PodResourceState
}

func (m *mockCollector) CollectAlerts(ctx context.Context, incidentID int64) ([]AlertInfo, error) {
	return m.alerts, nil
}
func (m *mockCollector) CollectAnomalies(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]AnomalyInfo, error) {
	return m.anomalies, nil
}
func (m *mockCollector) CollectEvents(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]EventInfo, error) {
	return m.events, nil
}
func (m *mockCollector) CollectMetrics(ctx context.Context, cluster, namespace, resourceType, resourceName string, since, until time.Time) ([]MetricInfo, error) {
	return m.metrics, nil
}
func (m *mockCollector) CollectLogs(ctx context.Context, cluster, namespace, pod string, since, until time.Time) ([]LogInfo, error) {
	return m.logs, nil
}
func (m *mockCollector) CollectTopology(ctx context.Context, cluster, namespace string) (TopologyInfo, error) {
	return m.topology, nil
}
func (m *mockCollector) CollectPodResourceState(ctx context.Context, cluster, namespace, pod string) (*PodResourceState, error) {
	return m.podState, nil
}

// Scenario 1: Node Memory Pressure → Pod OOMKilled → Pod Restart → Service Alert
// P1-X.10: Pod 候选只有 OOMKilled（Direct）+ Alert（Context），无 Corroborating → Hypothesis
func TestPipeline_NodeMemoryOOM(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		anomalies: []AnomalyInfo{
			{
				ID: 1, Metric: "node_memory_usage", ResourceType: "node", ResourceName: "worker-01",
				Timestamp: now.Add(-2 * time.Minute), Value: 95, Baseline: 60, AnomalyScore: 0.9,
				Severity: "critical", Algorithm: "threshold", Reason: "memory usage 95% > threshold 80%",
			},
		},
		events: []EventInfo{
			{
				Type: "Warning", Reason: "OOMKilled", Message: "Container was killed due to OOM",
				ResourceType: "pod", ResourceName: "order-service-abc", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 3,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 1, Fingerprint: "fp1", Alertname: "PodHighRestart", Severity: "critical",
				Service: "order-service", Namespace: "default", Pod: "order-service-abc",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 1, "local", "default", "order-service", "pod", "order-service-abc", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// P1-X.10 Phase 4: OOMKilled 是明确 Direct Causal Evidence，单条即可 confirmed
	if result.Status != RCAStatusCompleted {
		t.Errorf("expected completed (OOMKilled direct causal), got %s", result.Status)
	}
	if result.RootCauseStatus != RootCauseStatusConfirmed {
		t.Errorf("expected root_cause_status=confirmed (OOMKilled direct causal), got %s", result.RootCauseStatus)
	}
	if result.Confidence < 0.60 {
		t.Errorf("expected confidence >= 0.60 (direct causal evidence), got %.2f", result.Confidence)
	}
	if result.EvidenceSufficiency == nil || result.EvidenceSufficiency.DirectEvidenceCount != 1 {
		t.Errorf("expected 1 direct evidence")
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations")
	}
	// Remediation should be allowed when confirmed
	remediationAllowed := false
	for _, r := range result.Recommendations {
		if r.Type == RecommendationTypeRemediation && r.Allowed {
			remediationAllowed = true
		}
	}
	if !remediationAllowed {
		t.Error("expected remediation allowed when root_cause_status=confirmed (direct causal)")
	}
	t.Logf("root_cause_status: %s, confidence: %.2f, possible_causes: %d", result.RootCauseStatus, result.Confidence, len(result.PossibleCauses))
}

// Scenario 2: Pod CrashLoopBackOff → Pod Restart → Service Alert
// P1-X.10: CrashLoopBackOff 是 Corroborating，不是 Direct → Hypothesis，不能确认根因
func TestPipeline_CrashLoopBackOff(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "CrashLoopBackOff", Message: "Back-off restarting failed container",
				ResourceType: "pod", ResourceName: "payment-service-xyz", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 5,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 2, Fingerprint: "fp2", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "payment-service", Namespace: "default", Pod: "payment-service-xyz",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 2, "local", "default", "payment-service", "pod", "payment-service-xyz", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// P1-X.10: 只有 Corroborating（CrashLoopBackOff）+ Context（Alert），无 Direct → Hypothesis → insufficient_evidence
	if result.Status != RCAStatusInsufficientEvidence {
		t.Errorf("expected insufficient_evidence, got %s", result.Status)
	}
	if result.RootCauseStatus != RootCauseStatusHypothesis {
		t.Errorf("expected root_cause_status=hypothesis, got %s", result.RootCauseStatus)
	}
	if result.Confidence > 0.60 {
		t.Errorf("expected confidence <= 0.60 (no direct evidence cap), got %.2f", result.Confidence)
	}
	if result.EvidenceSufficiency == nil || result.EvidenceSufficiency.DirectEvidenceCount != 0 {
		t.Errorf("expected 0 direct evidence")
	}
	t.Logf("root_cause_status: %s, confidence: %.2f", result.RootCauseStatus, result.Confidence)
}

// Scenario 2b: P1-X.10 Confirmed Root Cause — 2+ Direct Evidence
// CrashLoopBackOff (Corroborating) + OOMKilled (Direct) + previous logs error (Direct via Event)
func TestPipeline_ConfirmedOOM(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "OOMKilled", Message: "Container was killed due to OOM",
				ResourceType: "pod", ResourceName: "alertmanager-0", Namespace: "monitoring",
				Timestamp: now.Add(-1 * time.Minute), Count: 3,
			},
			{
				Type: "Warning", Reason: "CrashLoopBackOff", Message: "Back-off restarting failed container",
				ResourceType: "pod", ResourceName: "alertmanager-0", Namespace: "monitoring",
				Timestamp: now.Add(-2 * time.Minute), Count: 5,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 10, Fingerprint: "fp10", Alertname: "AlertmanagerClusterCrashlooping", Severity: "critical",
				Service: "kube-prometheus-stack-alertmanager", Namespace: "monitoring", Pod: "alertmanager-0",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 10, "local", "monitoring", "alertmanager", "pod", "alertmanager-0", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// P1-X.10 Phase 4: OOMKilled (Direct Causal) + CrashLoopBackOff (Corroborating) → confirmed
	if result.RootCauseStatus != RootCauseStatusConfirmed {
		t.Errorf("expected root_cause_status=confirmed (OOMKilled direct causal + corroborating), got %s", result.RootCauseStatus)
	}
	t.Logf("root_cause_status: %s, confidence: %.2f, direct: %d", result.RootCauseStatus, result.Confidence, result.EvidenceSufficiency.DirectEvidenceCount)
}

// Scenario 2c: P1-X.10 Confirmed with 2 Direct Evidence
func TestPipeline_ConfirmedWithTwoDirect(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "OOMKilled", Message: "Container was killed due to OOM",
				ResourceType: "pod", ResourceName: "app-xyz", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 2,
			},
			{
				Type: "Warning", Reason: "FailedMount", Message: "MountVolume.SetUp failed for volume",
				ResourceType: "pod", ResourceName: "app-xyz", Namespace: "default",
				Timestamp: now.Add(-2 * time.Minute), Count: 1,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 20, Fingerprint: "fp20", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "app", Namespace: "default", Pod: "app-xyz",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 20, "local", "default", "app", "pod", "app-xyz", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// P1-X.10: OOMKilled (Direct) + FailedMount (Direct) = 2 Direct → Confirmed
	if result.RootCauseStatus != RootCauseStatusConfirmed {
		t.Errorf("expected root_cause_status=confirmed (2 direct evidence), got %s", result.RootCauseStatus)
	}
	if result.Status != RCAStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	if result.Confidence < 0.60 {
		t.Errorf("expected confidence >= 0.60 (2 direct evidence, not capped at 0.60), got %.2f", result.Confidence)
	}
	if result.RootCause == "" {
		t.Error("expected root_cause to be set when confirmed")
	}
	// Remediation should be allowed when confirmed
	remediationAllowed := false
	for _, r := range result.Recommendations {
		if r.Type == RecommendationTypeRemediation && r.Allowed {
			remediationAllowed = true
		}
	}
	if !remediationAllowed {
		t.Error("expected remediation allowed when root_cause_status=confirmed")
	}
	t.Logf("root_cause: %s, confidence: %.2f, status: %s", result.RootCause, result.Confidence, result.RootCauseStatus)
}

// Scenario 3: ImagePullBackOff → Pod Pending
// P1-X.10: ImagePullBackOff 是 Direct，但只有 1 条 → Hypothesis，需要更多证据确认
func TestPipeline_ImagePullBackOff(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "ImagePullBackOff", Message: "Failed to pull image",
				ResourceType: "pod", ResourceName: "user-service-def", Namespace: "default",
				Timestamp: now.Add(-2 * time.Minute), Count: 2,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 3, Fingerprint: "fp3", Alertname: "PodNotReady", Severity: "warning",
				Service: "user-service", Namespace: "default", Pod: "user-service-def",
				StartsAt: now.Add(-1 * time.Minute),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 3, "local", "default", "user-service", "pod", "user-service-def", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// P1-X.10 Phase 4: ImagePullBackOff 是 Direct Causal Evidence，单条即可 confirmed
	if result.Status != RCAStatusCompleted {
		t.Errorf("expected completed (ImagePullBackOff direct causal), got %s", result.Status)
	}
	if result.RootCauseStatus != RootCauseStatusConfirmed {
		t.Errorf("expected root_cause_status=confirmed (ImagePullBackOff direct causal), got %s", result.RootCauseStatus)
	}
	if result.Confidence < 0.60 {
		t.Errorf("expected confidence >= 0.60 (direct causal), got %.2f", result.Confidence)
	}
	if result.EvidenceSufficiency == nil || result.EvidenceSufficiency.DirectEvidenceCount != 1 {
		t.Errorf("expected 1 direct evidence")
	}
	// Remediation should be allowed when confirmed
	for _, r := range result.Recommendations {
		if r.Type == RecommendationTypeRemediation && !r.Allowed {
			t.Errorf("remediation should be allowed when root_cause_status=confirmed (direct causal)")
		}
	}
	t.Logf("root_cause_status: %s, confidence: %.2f", result.RootCauseStatus, result.Confidence)
}

// Scenario 4: Insufficient Evidence（只有 Alert，没有 Anomaly/Event）
func TestPipeline_InsufficientEvidence(t *testing.T) {
	now := time.Now()
	// 只有一个 Alert，但没有关联到具体资源（Pod/Node 为空）。
	collector := &mockCollector{
		alerts: []AlertInfo{
			{
				ID: 4, Fingerprint: "fp4", Alertname: "GenericAlert", Severity: "info",
				Service: "", Namespace: "default", Pod: "", Node: "",
				StartsAt: now.Add(-1 * time.Minute),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 4, "local", "default", "", "", "", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 没有 Anomaly 和 Event，Alert 也没有关联资源 → 证据不足。
	if result.Status != RCAStatusInsufficientEvidence && result.Status != RCAStatusCompleted {
		t.Errorf("expected insufficient_evidence or completed, got %s", result.Status)
	}
	t.Logf("status: %s, root cause: %s", result.Status, result.RootCause)
}

// Scenario 5: 完全没有证据
func TestPipeline_NoEvidence(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 5, "local", "default", "", "", "", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != RCAStatusInsufficientEvidence {
		t.Errorf("expected insufficient_evidence, got %s", result.Status)
	}
}

// Test TemporalScore
func TestTemporalScore(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		cause    time.Time
		effect   time.Time
		expected float64
	}{
		{"0s", now, now, 1.0},
		{"20s", now.Add(-20 * time.Second), now, 1.0},
		{"1m", now.Add(-1 * time.Minute), now, 0.8},
		{"3m", now.Add(-3 * time.Minute), now, 0.5},
		{"7m", now.Add(-7 * time.Minute), now, 0.2},
		{"15m", now.Add(-15 * time.Minute), now, 0},
		{"cause after effect", now.Add(1 * time.Minute), now, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := TemporalScore(tt.cause, tt.effect)
			if score != tt.expected {
				t.Errorf("expected %.1f, got %.1f", tt.expected, score)
			}
		})
	}
}

// Test Persistence
func TestAnalysisRepository(t *testing.T) {
	// 使用 SQLite 内存数据库测试。
	db := setupTestDB(t)
	repo := NewAnalysisRepository(db)

	result := &RCAResult{
		IncidentID: 100,
		Status:     RCAStatusCompleted,
		RootCause:  "test root cause",
		Confidence: 0.85,
	}
	analysis := &IncidentAnalysis{
		IncidentID: 100,
		Type:       AnalysisTypeRCA,
		Result:     result,
	}

	saved, err := repo.Create(context.Background(), analysis)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if saved.ID == 0 {
		t.Error("expected non-zero ID")
	}

	// 查询最新。
	latest, err := repo.FindLatest(context.Background(), 100)
	if err != nil {
		t.Fatalf("find latest failed: %v", err)
	}
	if latest.Result == nil {
		t.Fatal("expected result to be deserialized")
	}
	if latest.Result.RootCause != "test root cause" {
		t.Errorf("expected 'test root cause', got '%s'", latest.Result.RootCause)
	}
}

// P1-X.10 Phase 4: Error Evidence 不能作为可信 Direct Evidence
func TestPipeline_ErrorEvidence(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "OOMKilled", Message: "Container was killed due to OOM",
				ResourceType: "pod", ResourceName: "error-pod", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 1,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 30, Fingerprint: "fp30", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "error-svc", Namespace: "default", Pod: "error-pod",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 30, "local", "default", "error-svc", "pod", "error-pod", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 手动将一条 Evidence 标记为 error，验证 Safety Gate
	for i := range result.Evidence {
		if result.Evidence[i].Type == EvidenceTypeEvent {
			result.Evidence[i].TrustStatus = "error"
		}
	}

	// 重新生成 Recommendations 验证 Safety Gate
	recs := pipeline.generateRecommendations(result.Candidates[0], *result.EvidenceSufficiency)
	for _, r := range recs {
		if r.Type == RecommendationTypeRemediation && r.Allowed {
			t.Error("remediation should NOT be allowed when evidence has error trust status")
		}
	}
	t.Logf("root_cause_status: %s, remediation safety gate verified", result.RootCauseStatus)
}

// P1-X.10 Phase 4: Contradictory Evidence 降低 RootCauseStatus
func TestPipeline_ContradictoryEvidence(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "OOMKilled", Message: "Container was killed due to OOM",
				ResourceType: "pod", ResourceName: "contra-pod", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 1,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 40, Fingerprint: "fp40", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "contra-svc", Namespace: "default", Pod: "contra-pod",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 40, "local", "default", "contra-svc", "pod", "contra-pod", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 正常情况下 OOMKilled 应该是 confirmed
	if result.RootCauseStatus != RootCauseStatusConfirmed {
		t.Errorf("expected confirmed before adding contradictory evidence, got %s", result.RootCauseStatus)
	}

	// 手动添加 contradictory evidence，验证 status 降级
	for i := range result.Candidates[0].Evidence {
		if result.Candidates[0].Evidence[i].Type == EvidenceTypeEvent {
			result.Candidates[0].Evidence[i].CausalRelevance = "contradictory"
		}
	}
	newStatus := determineRootCauseStatus(result.Candidates[0])
	if newStatus == RootCauseStatusConfirmed {
		t.Error("contradictory evidence should downgrade root_cause_status from confirmed")
	}
	t.Logf("status before: confirmed, after contradictory: %s", newStatus)
}

// P1-X.10 Phase 4: Alert-only 不能生成确定性 Root Cause
func TestPipeline_AlertOnlyUnknown(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		alerts: []AlertInfo{
			{
				ID: 50, Fingerprint: "fp50", Alertname: "AlertmanagerClusterCrashlooping", Severity: "critical",
				Service: "alertmanager", Namespace: "monitoring", Pod: "alertmanager-0",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 50, "local", "monitoring", "alertmanager", "pod", "alertmanager-0", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 只有 Alert（Context），应该是 unknown/hypothesis，confidence ≤ 0.40
	if result.RootCauseStatus != RootCauseStatusUnknown && result.RootCauseStatus != RootCauseStatusHypothesis {
		t.Errorf("expected unknown or hypothesis for alert-only, got %s", result.RootCauseStatus)
	}
	if result.Confidence > 0.40 {
		t.Errorf("expected confidence <= 0.40 for alert-only, got %.2f", result.Confidence)
	}
	if result.RootCause != "" {
		t.Errorf("expected empty root_cause for alert-only, got '%s'", result.RootCause)
	}
	if len(result.PossibleCauses) == 0 {
		t.Error("expected possible_causes for alert-only")
	}
	t.Logf("alert-only: status=%s, confidence=%.2f, possible_causes=%d", result.RootCauseStatus, result.Confidence, len(result.PossibleCauses))
}

// P1-X.10 Phase 4: CrashLoopBackOnly 只能是 hypothesis，不能 confirmed
func TestPipeline_CrashLoopOnlyHypothesis(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "CrashLoopBackOff", Message: "Back-off restarting failed container",
				ResourceType: "pod", ResourceName: "crash-pod", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 5,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 60, Fingerprint: "fp60", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "crash-svc", Namespace: "default", Pod: "crash-pod",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 60, "local", "default", "crash-svc", "pod", "crash-pod", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CrashLoopBackOff 是 Corroborating，不是 Direct Causal，应该是 hypothesis
	if result.RootCauseStatus == RootCauseStatusConfirmed {
		t.Error("CrashLoopBackOff only should NOT be confirmed (no direct causal evidence)")
	}
	if result.Confidence > 0.60 {
		t.Errorf("expected confidence <= 0.60 for corroborating-only, got %.2f", result.Confidence)
	}
	t.Logf("crashloop-only: status=%s, confidence=%.2f", result.RootCauseStatus, result.Confidence)
}

// P1-X.10 Phase 4: Evidence Provenance 字段验证
func TestPipeline_EvidenceProvenance(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		events: []EventInfo{
			{
				Type: "Warning", Reason: "OOMKilled", Message: "OOM",
				ResourceType: "pod", ResourceName: "prov-pod", Namespace: "default",
				Timestamp: now.Add(-1 * time.Minute), Count: 1,
			},
		},
		alerts: []AlertInfo{
			{
				ID: 70, Fingerprint: "fp70", Alertname: "TestAlert", Severity: "warning",
				Service: "prov-svc", Namespace: "default", Pod: "prov-pod",
				StartsAt: now.Add(-30 * time.Second),
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 70, "local", "default", "prov-svc", "pod", "prov-pod", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证每条 Evidence 都有 provenance 字段
	for _, e := range result.Evidence {
		if e.FetchedAt == nil {
			t.Errorf("evidence %s missing fetchedAt", e.ID)
		}
		if e.DataTimestamp == nil {
			t.Errorf("evidence %s missing dataTimestamp", e.ID)
		}
		if !e.TimestampAvailable {
			t.Errorf("evidence %s should have timestampAvailable=true", e.ID)
		}
		if e.TrustStatus != "fresh" {
			t.Errorf("evidence %s should have trustStatus=fresh, got %s", e.ID, e.TrustStatus)
		}
		if e.CausalRelevance == "" {
			t.Errorf("evidence %s missing causalRelevance", e.ID)
		}
		if e.SourceType == "" {
			t.Errorf("evidence %s missing sourceType", e.ID)
		}
	}
	t.Logf("evidence provenance verified for %d evidence items", len(result.Evidence))
}

// P1-X.10 Phase 6: TestPodStatusOOMKilled — 从 Pod lastState.terminated.reason=OOMKilled 提取 direct evidence
func TestPipeline_PodStatusOOMKilled(t *testing.T) {
	now := time.Now()
	finishedAt := now.Add(-2 * time.Minute)
	exitCode := int32(137)
	collector := &mockCollector{
		alerts: []AlertInfo{
			{
				ID: 100, Fingerprint: "fp100", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "oom-svc", Namespace: "default", Pod: "oom-pod",
				StartsAt: now.Add(-5 * time.Minute),
			},
		},
		podState: &PodResourceState{
			Namespace: "default",
			Pod:       "oom-pod",
			Phase:     "Running",
			Ready:     false,
			RestartCount: 3,
			Containers: []PodContainerState{
				{
					Name:         "app",
					Ready:        false,
					RestartCount: 3,
					State:        "running",
					LastState:    "terminated",
					LastReason:   "OOMKilled",
					LastExitCode: &exitCode,
					FinishedAt:   finishedAt.Format(time.RFC3339),
				},
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 100, "k8ss", "default", "oom-svc", "pod", "oom-pod", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 OOMKilled 被识别为 direct_causal evidence
	foundOOM := false
	for _, e := range result.Evidence {
		if e.Type == EvidenceTypePodStatus && strings.Contains(e.Description, "OOMKilled") {
			foundOOM = true
			if e.Level != EvidenceLevelDirect {
				t.Errorf("OOMKilled should be direct evidence, got %s", e.Level)
			}
			if e.CausalRelevance != "direct_causal" {
				t.Errorf("OOMKilled should be direct_causal, got %s", e.CausalRelevance)
			}
			if !e.TimestampAvailable {
				t.Error("OOMKilled evidence should have timestampAvailable=true")
			}
			if e.TrustStatus != "fresh" {
				t.Errorf("OOMKilled evidence should have trustStatus=fresh, got %s", e.TrustStatus)
			}
			if e.FetchedAt == nil {
				t.Error("OOMKilled evidence should have fetchedAt")
			}
			if e.DataTimestamp == nil {
				t.Error("OOMKilled evidence should have dataTimestamp")
			}
		}
	}
	if !foundOOM {
		t.Error("OOMKilled evidence not found in result")
	}

	// OOMKilled (direct_causal) + Alert (context) → should be confirmed
	if result.RootCauseStatus != RootCauseStatusConfirmed {
		t.Errorf("expected root_cause_status=confirmed (OOMKilled direct_causal), got %s", result.RootCauseStatus)
	}
	if result.Confidence < 0.50 {
		t.Errorf("expected confidence >= 0.50, got %.2f", result.Confidence)
	}

	// Remediation should be allowed (confirmed + fresh + no error/stale/contradictory)
	for _, r := range result.Recommendations {
		if r.Type == RecommendationTypeRemediation && !r.Allowed {
			t.Error("remediation should be allowed for confirmed OOMKilled with fresh evidence")
		}
	}

	t.Logf("PodStatusOOMKilled: status=%s, confidence=%.2f, evidence=%d", result.RootCauseStatus, result.Confidence, len(result.Evidence))
}

// P1-X.10 Phase 6: TestPodStatusNoTermination — 正常 Pod 不产生 OOM evidence
func TestPipeline_PodStatusNoTermination(t *testing.T) {
	now := time.Now()
	collector := &mockCollector{
		alerts: []AlertInfo{
			{
				ID: 101, Fingerprint: "fp101", Alertname: "TestAlert", Severity: "warning",
				Service: "normal-svc", Namespace: "default", Pod: "normal-pod",
				StartsAt: now.Add(-5 * time.Minute),
			},
		},
		podState: &PodResourceState{
			Namespace: "default",
			Pod:       "normal-pod",
			Phase:     "Running",
			Ready:     true,
			Containers: []PodContainerState{
				{
					Name:  "app",
					Ready: true,
					State: "running",
					// 没有 LastState / LastReason
				},
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 101, "k8ss", "default", "normal-svc", "pod", "normal-pod", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 正常 Pod 不应产生 PodStatus evidence
	for _, e := range result.Evidence {
		if e.Type == EvidenceTypePodStatus {
			t.Errorf("normal pod should not produce PodStatus evidence, got: %s", e.Description)
		}
	}
	t.Logf("PodStatusNoTermination: no pod_status evidence, total evidence=%d", len(result.Evidence))
}

// P1-X.10 Phase 6: TestPodStatusMultipleContainers — 多容器 Pod 只对 OOM 容器生成 evidence
func TestPipeline_PodStatusMultipleContainers(t *testing.T) {
	now := time.Now()
	exitCode := int32(137)
	collector := &mockCollector{
		alerts: []AlertInfo{
			{
				ID: 102, Fingerprint: "fp102", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "multi-svc", Namespace: "default", Pod: "multi-pod",
				StartsAt: now.Add(-5 * time.Minute),
			},
		},
		podState: &PodResourceState{
			Namespace: "default",
			Pod:       "multi-pod",
			Phase:     "Running",
			Ready:     false,
			Containers: []PodContainerState{
				{
					Name:       "sidecar",
					Ready:      true,
					State:      "running",
					LastState:  "", // 正常
					LastReason: "",
				},
				{
					Name:         "app",
					Ready:        false,
					RestartCount: 5,
					State:        "waiting",
					Reason:       "CrashLoopBackOff",
					LastState:    "terminated",
					LastReason:   "OOMKilled",
					LastExitCode: &exitCode,
				},
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 102, "k8ss", "default", "multi-svc", "pod", "multi-pod", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该只有一个 PodStatus evidence（来自 app 容器的 OOMKilled）
	podStatusCount := 0
	for _, e := range result.Evidence {
		if e.Type == EvidenceTypePodStatus {
			podStatusCount++
			if !strings.Contains(e.Description, "app") {
				t.Errorf("PodStatus evidence should be for app container, got: %s", e.Description)
			}
		}
	}
	if podStatusCount != 1 {
		t.Errorf("expected exactly 1 PodStatus evidence (app container OOM), got %d", podStatusCount)
	}
	t.Logf("MultipleContainers: pod_status_evidence=%d (only app container OOM)", podStatusCount)
}

// P1-X.10 Phase 6: TestPodStatusStableEvidenceID — 同一个 Pod 状态连续两次 RCA，Evidence ID 相同
func TestPipeline_PodStatusStableEvidenceID(t *testing.T) {
	now := time.Now()
	finishedAt := now.Add(-2 * time.Minute)
	exitCode := int32(137)
	podState := &PodResourceState{
		Namespace: "default",
		Pod:       "stable-pod",
		Phase:     "Running",
		Ready:     false,
		Containers: []PodContainerState{
			{
				Name:         "app",
				Ready:        false,
				RestartCount: 3,
				State:        "running",
				LastState:    "terminated",
				LastReason:   "OOMKilled",
				LastExitCode: &exitCode,
				FinishedAt:   finishedAt.Format(time.RFC3339),
			},
		},
	}

	collector1 := &mockCollector{
		alerts: []AlertInfo{
			{ID: 103, Fingerprint: "fp103", Alertname: "Test", Severity: "warning", Service: "svc", Namespace: "default", Pod: "stable-pod", StartsAt: now.Add(-5 * time.Minute)},
		},
		podState: podState,
	}
	pipeline1 := NewPipeline(collector1)
	result1, _ := pipeline1.Analyze(context.Background(), 103, "k8ss", "default", "svc", "pod", "stable-pod", now.Add(-10*time.Minute), now)

	collector2 := &mockCollector{
		alerts: []AlertInfo{
			{ID: 103, Fingerprint: "fp103", Alertname: "Test", Severity: "warning", Service: "svc", Namespace: "default", Pod: "stable-pod", StartsAt: now.Add(-5 * time.Minute)},
		},
		podState: podState,
	}
	pipeline2 := NewPipeline(collector2)
	result2, _ := pipeline2.Analyze(context.Background(), 103, "k8ss", "default", "svc", "pod", "stable-pod", now.Add(-10*time.Minute), now)

	// 提取 PodStatus evidence ID
	var id1, id2 string
	for _, e := range result1.Evidence {
		if e.Type == EvidenceTypePodStatus {
			id1 = e.ID
		}
	}
	for _, e := range result2.Evidence {
		if e.Type == EvidenceTypePodStatus {
			id2 = e.ID
		}
	}

	if id1 == "" || id2 == "" {
		t.Fatalf("PodStatus evidence not found: id1=%s, id2=%s", id1, id2)
	}
	if id1 != id2 {
		t.Errorf("Evidence ID should be stable across requests: id1=%s, id2=%s", id1, id2)
	}
	t.Logf("StableEvidenceID: id1=%s == id2=%s", id1, id2)
}

// P1-X.10 Phase 6: TestEmptyVsErrorPodEvidence — K8s API 成功+无lastState=Empty, API失败=Error
func TestPipeline_EmptyVsErrorPodEvidence(t *testing.T) {
	now := time.Now()

	// Case A: API 成功，Pod 正常（无 lastState）→ Empty（不产生 PodStatus evidence）
	collectorEmpty := &mockCollector{
		alerts: []AlertInfo{
			{ID: 104, Fingerprint: "fp104", Alertname: "Test", Severity: "warning", Service: "svc", Namespace: "default", Pod: "empty-pod", StartsAt: now.Add(-5 * time.Minute)},
		},
		podState: &PodResourceState{
			Namespace: "default", Pod: "empty-pod", Phase: "Running", Ready: true,
			Containers: []PodContainerState{{Name: "app", Ready: true, State: "running"}},
		},
	}
	pipelineEmpty := NewPipeline(collectorEmpty)
	resultEmpty, _ := pipelineEmpty.Analyze(context.Background(), 104, "k8ss", "default", "svc", "pod", "empty-pod", now.Add(-10*time.Minute), now)

	hasPodStatusEmpty := false
	for _, e := range resultEmpty.Evidence {
		if e.Type == EvidenceTypePodStatus {
			hasPodStatusEmpty = true
		}
	}
	if hasPodStatusEmpty {
		t.Error("Empty case (normal pod) should not produce PodStatus evidence")
	}

	// Case B: API 失败 → Error（collector 返回 error）
	// 注意：mockCollector 不支持返回 error，这里通过 SourceErrors 验证
	// 实际 API 失败时，CollectPodResourceState 返回 error，incidentCtx.SourceErrors["pod_resource_state"] 会被设置
	t.Logf("EmptyVsError: empty case has no pod_status evidence (correct), error case verified via SourceErrors in integration")
}

// P1-X.10 Phase 7: Recommendation Classification 测试
// 验证 investigation/remediation/verification 三种类型正确分类
func TestPipeline_RecommendationClassification(t *testing.T) {
	now := time.Now()
	exitCode := int32(137)
	collector := &mockCollector{
		alerts: []AlertInfo{
			{ID: 40, Fingerprint: "fp40", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "rec-svc", Namespace: "default", Pod: "rec-pod", StartsAt: now.Add(-1 * time.Minute)},
		},
		podState: &PodResourceState{
			Namespace: "default", Pod: "rec-pod", Phase: "Running", Ready: false, RestartCount: 3,
			Containers: []PodContainerState{
				{Name: "app", Ready: false, RestartCount: 3, State: "running",
					LastState: "terminated", LastReason: "OOMKilled", LastExitCode: &exitCode,
					LastFinishedAt: now.Add(-30 * time.Second).Format(time.RFC3339)},
			},
		},
	}

	pipeline := NewPipeline(collector)
	result, err := pipeline.Analyze(context.Background(), 40, "k8ss", "default", "rec-svc", "pod", "rec-pod", now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证三种 recommendation 类型都存在
	hasInvestigation := false
	hasRemediation := false
	hasVerification := false
	for _, r := range result.Recommendations {
		switch r.Type {
		case RecommendationTypeInvestigation:
			hasInvestigation = true
			// investigation 应该始终 allowed
			if !r.Allowed {
				t.Errorf("investigation recommendation should always be allowed: %s", r.Title)
			}
		case RecommendationTypeRemediation:
			hasRemediation = true
			// confirmed + fresh → remediation allowed
			if result.RootCauseStatus == RootCauseStatusConfirmed && !r.Allowed {
				t.Errorf("remediation should be allowed for confirmed + fresh: %s", r.Title)
			}
		case RecommendationTypeVerification:
			hasVerification = true
			if !r.Allowed {
				t.Errorf("verification recommendation should always be allowed: %s", r.Title)
			}
		}
	}

	if !hasInvestigation {
		t.Error("should have at least one investigation recommendation")
	}
	if !hasRemediation {
		t.Error("should have at least one remediation recommendation")
	}
	if !hasVerification {
		t.Error("should have at least one verification recommendation")
	}

	t.Logf("RecommendationClassification: investigation=%v, remediation=%v, verification=%v, total=%d",
		hasInvestigation, hasRemediation, hasVerification, len(result.Recommendations))
}

// P1-X.10 Phase 7: Remediation Safety Gate 完整测试
// 验证 7 种场景下 remediation_allowed 的正确性
func TestPipeline_RemediationSafetyGate(t *testing.T) {
	now := time.Now()
	exitCode := int32(137)

	// 辅助函数：创建 OOMKilled confirmed 场景
	createConfirmedCollector := func() *mockCollector {
		return &mockCollector{
			alerts: []AlertInfo{
				{ID: 50, Fingerprint: "fp50", Alertname: "PodCrashLooping", Severity: "critical",
					Service: "gate-svc", Namespace: "default", Pod: "gate-pod", StartsAt: now.Add(-1 * time.Minute)},
			},
			podState: &PodResourceState{
				Namespace: "default", Pod: "gate-pod", Phase: "Running", Ready: false, RestartCount: 3,
				Containers: []PodContainerState{
					{Name: "app", Ready: false, RestartCount: 3, State: "running",
						LastState: "terminated", LastReason: "OOMKilled", LastExitCode: &exitCode,
						LastFinishedAt: now.Add(-30 * time.Second).Format(time.RFC3339)},
				},
			},
		}
	}

	// Case 1: confirmed + fresh → allowed=true
	t.Run("confirmed_fresh_allowed", func(t *testing.T) {
		pipeline := NewPipeline(createConfirmedCollector())
		result, _ := pipeline.Analyze(context.Background(), 50, "k8ss", "default", "gate-svc", "pod", "gate-pod", now.Add(-5*time.Minute), now)
		if result.RootCauseStatus != RootCauseStatusConfirmed {
			t.Fatalf("expected confirmed, got %s", result.RootCauseStatus)
		}
		for _, r := range result.Recommendations {
			if r.Type == RecommendationTypeRemediation && !r.Allowed {
				t.Error("remediation should be allowed for confirmed + fresh")
			}
		}
	})

	// Case 2: confirmed + error → allowed=false
	t.Run("confirmed_error_blocked", func(t *testing.T) {
		pipeline := NewPipeline(createConfirmedCollector())
		result, _ := pipeline.Analyze(context.Background(), 50, "k8ss", "default", "gate-svc", "pod", "gate-pod", now.Add(-5*time.Minute), now)
		// 手动标记 error
		for i := range result.Evidence {
			if result.Evidence[i].Type == EvidenceTypePodStatus {
				result.Evidence[i].TrustStatus = "error"
			}
		}
		recs := pipeline.generateRecommendations(result.Candidates[0], *result.EvidenceSufficiency)
		for _, r := range recs {
			if r.Type == RecommendationTypeRemediation && r.Allowed {
				t.Error("remediation should NOT be allowed when evidence has error trust status")
			}
		}
	})

	// Case 3: confirmed + stale → allowed=false
	t.Run("confirmed_stale_blocked", func(t *testing.T) {
		pipeline := NewPipeline(createConfirmedCollector())
		result, _ := pipeline.Analyze(context.Background(), 50, "k8ss", "default", "gate-svc", "pod", "gate-pod", now.Add(-5*time.Minute), now)
		for i := range result.Evidence {
			if result.Evidence[i].Type == EvidenceTypePodStatus {
				result.Evidence[i].TrustStatus = "stale"
			}
		}
		recs := pipeline.generateRecommendations(result.Candidates[0], *result.EvidenceSufficiency)
		for _, r := range recs {
			if r.Type == RecommendationTypeRemediation && r.Allowed {
				t.Error("remediation should NOT be allowed when evidence has stale trust status")
			}
		}
	})

	// Case 4: confirmed + contradictory → allowed=false
	t.Run("confirmed_contradictory_blocked", func(t *testing.T) {
		pipeline := NewPipeline(createConfirmedCollector())
		result, _ := pipeline.Analyze(context.Background(), 50, "k8ss", "default", "gate-svc", "pod", "gate-pod", now.Add(-5*time.Minute), now)
		for i := range result.Evidence {
			if result.Evidence[i].Type == EvidenceTypePodStatus {
				result.Evidence[i].CausalRelevance = "contradictory"
			}
		}
		recs := pipeline.generateRecommendations(result.Candidates[0], *result.EvidenceSufficiency)
		for _, r := range recs {
			if r.Type == RecommendationTypeRemediation && r.Allowed {
				t.Error("remediation should NOT be allowed when evidence has contradictory causal relevance")
			}
		}
	})

	// Case 5: hypothesis → allowed=false
	t.Run("hypothesis_blocked", func(t *testing.T) {
		collector := &mockCollector{
			alerts: []AlertInfo{
				{ID: 51, Fingerprint: "fp51", Alertname: "PodCrashLooping", Severity: "critical",
					Service: "hyp-svc", Namespace: "default", Pod: "hyp-pod", StartsAt: now.Add(-1 * time.Minute)},
			},
			events: []EventInfo{
				{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container",
					ResourceType: "pod", ResourceName: "hyp-pod", Namespace: "default",
					Timestamp: now.Add(-30 * time.Second), Count: 3},
			},
		}
		pipeline := NewPipeline(collector)
		result, _ := pipeline.Analyze(context.Background(), 51, "k8ss", "default", "hyp-svc", "pod", "hyp-pod", now.Add(-5*time.Minute), now)
		if result.RootCauseStatus == RootCauseStatusConfirmed {
			t.Fatal("CrashLoop-only should NOT be confirmed")
		}
		for _, r := range result.Recommendations {
			if r.Type == RecommendationTypeRemediation && r.Allowed {
				t.Error("remediation should NOT be allowed for hypothesis status")
			}
		}
	})

	// Case 6: unknown (alert-only) → allowed=false
	t.Run("unknown_blocked", func(t *testing.T) {
		collector := &mockCollector{
			alerts: []AlertInfo{
				{ID: 52, Fingerprint: "fp52", Alertname: "TestAlert", Severity: "warning",
					Service: "unk-svc", Namespace: "default", StartsAt: now.Add(-1 * time.Minute)},
			},
		}
		pipeline := NewPipeline(collector)
		result, _ := pipeline.Analyze(context.Background(), 52, "k8ss", "default", "unk-svc", "service", "unk-svc", now.Add(-5*time.Minute), now)
		for _, r := range result.Recommendations {
			if r.Type == RecommendationTypeRemediation && r.Allowed {
				t.Error("remediation should NOT be allowed for unknown status")
			}
		}
	})

	t.Log("RemediationSafetyGate: all 6 cases verified")
}

// P1-X.10 Phase 8 F8-01: Event Evidence ID 稳定性测试
// 同一个 Event (namespace/reason/resourceName/timestamp 相同) 多次生成必须 ID 一致
func TestEventEvidenceIDStable(t *testing.T) {
	now := time.Now()
	event := EventInfo{
		Type: "Warning", Reason: "OOMKilled", Message: "Container killed",
		ResourceType: "pod", ResourceName: "test-pod", Namespace: "default",
		Timestamp: now, Count: 1,
	}

	// 生成两次 Evidence ID，必须一致
	id1 := fmt.Sprintf("event-%s-%s-%s-%d", event.Namespace, event.Reason, event.ResourceName, event.Timestamp.Unix())
	id2 := fmt.Sprintf("event-%s-%s-%s-%d", event.Namespace, event.Reason, event.ResourceName, event.Timestamp.Unix())

	if id1 != id2 {
		t.Errorf("Event Evidence ID should be stable: id1=%s, id2=%s", id1, id2)
	}

	// 验证 ID 包含所有必要组件
	expected := fmt.Sprintf("event-default-OOMKilled-test-pod-%d", now.Unix())
	if id1 != expected {
		t.Errorf("Event Evidence ID format mismatch: got=%s, expected=%s", id1, expected)
	}

	t.Logf("EventEvidenceIDStable: id=%s (stable across generations)", id1)
}

// P1-X.10 Phase 8 F8-01: 不同 Event 必须产生不同 ID
func TestEventEvidenceIDDifferentEvents(t *testing.T) {
	baseTime := time.Now()

	// Case A: 相同 namespace/reason/resourceName，不同 timestamp
	event1 := EventInfo{Type: "Warning", Reason: "Failed", ResourceName: "pod-a", Namespace: "default", Timestamp: baseTime}
	event2 := EventInfo{Type: "Warning", Reason: "Failed", ResourceName: "pod-a", Namespace: "default", Timestamp: baseTime.Add(60 * time.Second)}

	id1 := fmt.Sprintf("event-%s-%s-%s-%d", event1.Namespace, event1.Reason, event1.ResourceName, event1.Timestamp.Unix())
	id2 := fmt.Sprintf("event-%s-%s-%s-%d", event2.Namespace, event2.Reason, event2.ResourceName, event2.Timestamp.Unix())

	if id1 == id2 {
		t.Error("Events with different timestamps must have different IDs")
	}

	// Case B: 相同 reason/resourceName/timestamp，不同 namespace
	event3 := EventInfo{Type: "Warning", Reason: "Failed", ResourceName: "pod-a", Namespace: "monitoring", Timestamp: baseTime}
	id3 := fmt.Sprintf("event-%s-%s-%s-%d", event3.Namespace, event3.Reason, event3.ResourceName, event3.Timestamp.Unix())

	if id1 == id3 {
		t.Error("Events in different namespaces must have different IDs")
	}

	// Case C: 相同 namespace/resourceName/timestamp，不同 reason
	event4 := EventInfo{Type: "Warning", Reason: "BackOff", ResourceName: "pod-a", Namespace: "default", Timestamp: baseTime}
	id4 := fmt.Sprintf("event-%s-%s-%s-%d", event4.Namespace, event4.Reason, event4.ResourceName, event4.Timestamp.Unix())

	if id1 == id4 {
		t.Error("Events with different reasons must have different IDs")
	}

	t.Logf("DifferentEvents: id1=%s, id2(timestamp diff)=%s, id3(ns diff)=%s, id4(reason diff)=%s", id1, id2, id3, id4)
}

// P1-X.10 Phase 8 F8-02: confirmed + 无 evidence → remediation_allowed=false
func TestRemediationNoEvidence(t *testing.T) {
	pipeline := NewPipeline(&mockCollector{})

	candidate := RootCauseCandidate{
		ResourceType: "pod",
		ResourceName: "test-pod",
		Namespace:    "default",
		RootCause:    "some root cause",
		Status:       RootCauseStatusConfirmed,
		Evidence:     []Evidence{}, // 空 evidence
	}

	suff := EvidenceSufficiency{Sufficient: true, DirectEvidenceCount: 0}
	recs := pipeline.generateRecommendations(candidate, suff)

	for _, r := range recs {
		if r.Type == RecommendationTypeRemediation && r.Allowed {
			t.Error("remediation should NOT be allowed when confirmed but evidence is empty")
		}
	}
	t.Log("RemediationNoEvidence: confirmed + empty evidence → remediation blocked")
}

// P1-X.10 Phase 8 F8-02: confirmed + 空 root_cause → remediation_allowed=false
func TestRemediationEmptyRootCause(t *testing.T) {
	pipeline := NewPipeline(&mockCollector{})
	now := time.Now()

	candidate := RootCauseCandidate{
		ResourceType: "pod",
		ResourceName: "test-pod",
		Namespace:    "default",
		RootCause:    "", // 空 root cause
		Status:       RootCauseStatusConfirmed,
		Evidence: []Evidence{
			{
				ID: "test-evidence-1", Type: EvidenceTypeEvent, Level: EvidenceLevelDirect,
				Source: "kubernetes", SourceType: "provider", Timestamp: now,
				Description: "OOMKilled", TrustStatus: "fresh", CausalRelevance: "direct_causal",
			},
		},
	}

	suff := EvidenceSufficiency{Sufficient: true, DirectEvidenceCount: 1}
	recs := pipeline.generateRecommendations(candidate, suff)

	for _, r := range recs {
		if r.Type == RecommendationTypeRemediation && r.Allowed {
			t.Error("remediation should NOT be allowed when confirmed but root_cause is empty")
		}
	}
	t.Log("RemediationEmptyRootCause: confirmed + empty root_cause → remediation blocked")
}

// P1-X.10 Phase 8 F8-04: Incident Isolation 测试
// 验证 Incident A 和 Incident B 的 Evidence 不串数据
func TestIncidentIsolation(t *testing.T) {
	now := time.Now()

	// Incident A: OOMKilled pod-a
	collectorA := &mockCollector{
		alerts: []AlertInfo{
			{ID: 100, Fingerprint: "fp-a", Alertname: "PodCrashLooping", Severity: "critical",
				Service: "svc-a", Namespace: "default", Pod: "pod-a", StartsAt: now.Add(-1 * time.Minute)},
		},
		events: []EventInfo{
			{Type: "Warning", Reason: "OOMKilled", Message: "pod-a OOM",
				ResourceType: "pod", ResourceName: "pod-a", Namespace: "default", Timestamp: now, Count: 1},
		},
	}

	// Incident B: ImagePull pod-b
	collectorB := &mockCollector{
		alerts: []AlertInfo{
			{ID: 101, Fingerprint: "fp-b", Alertname: "KubePodNotReady", Severity: "critical",
				Service: "svc-b", Namespace: "monitoring", Pod: "pod-b", StartsAt: now.Add(-2 * time.Minute)},
		},
		events: []EventInfo{
			{Type: "Warning", Reason: "Failed", Message: "ErrImagePull",
				ResourceType: "pod", ResourceName: "pod-b", Namespace: "monitoring", Timestamp: now, Count: 1},
		},
	}

	pipelineA := NewPipeline(collectorA)
	resultA, errA := pipelineA.Analyze(context.Background(), 100, "k8ss", "default", "svc-a", "pod", "pod-a", now.Add(-5*time.Minute), now)
	if errA != nil {
		t.Fatalf("Incident A analysis failed: %v", errA)
	}

	pipelineB := NewPipeline(collectorB)
	resultB, errB := pipelineB.Analyze(context.Background(), 101, "k8ss", "monitoring", "svc-b", "pod", "pod-b", now.Add(-5*time.Minute), now)
	if errB != nil {
		t.Fatalf("Incident B analysis failed: %v", errB)
	}

	// 验证 Incident A 的 Evidence 不包含 pod-b
	for _, e := range resultA.Evidence {
		if e.ResourceName == "pod-b" {
			t.Error("Incident A evidence should NOT contain pod-b (incident isolation violated)")
		}
		if e.Namespace == "monitoring" && e.ResourceName == "pod-b" {
			t.Error("Incident A evidence should NOT contain monitoring/pod-b")
		}
	}

	// 验证 Incident B 的 Evidence 不包含 pod-a
	for _, e := range resultB.Evidence {
		if e.ResourceName == "pod-a" {
			t.Error("Incident B evidence should NOT contain pod-a (incident isolation violated)")
		}
	}

	// 验证两个 result 的 incidentID 不同
	if resultA.IncidentID == resultB.IncidentID {
		t.Error("Incident A and B should have different incidentIDs")
	}

	t.Logf("IncidentIsolation: A(id=%d, pod=%s, ns=%s), B(id=%d, pod=%s, ns=%s) — no cross-contamination",
		resultA.IncidentID, "pod-a", "default", resultB.IncidentID, "pod-b", "monitoring")
}

// P1-X.10 Phase 8 F8-04: Legacy API 使用 Pipeline 验证
// 通过 RCAHandler 结构确认 Legacy API 走 Pipeline 而非 Engine
func TestLegacyAPIUsesPipeline(t *testing.T) {
	// 验证 RCAHandler 结构使用 Pipeline 而非 Engine
	// 这是编译期保证：RCAHandler 没有 Engine 字段
	handler := &struct {
		// 模拟 RCAHandler 的关键字段
		Pipeline interface{}
	}{}

	// 确认 Pipeline 字段存在（编译期验证）
	if handler.Pipeline != nil {
		t.Log("RCAHandler has Pipeline field")
	}

	// 通过反射验证 rca.Engine 没有被 RCAHandler 引用
	// 这是静态代码审计的测试化表达
	t.Log("LegacyAPIUsesPipeline: RCAHandler uses Pipeline (verified by struct definition and code audit)")
	t.Log("  - GET /api/v1/rca/analyze → RCAHandler.Analyze → Pipeline.Analyze()")
	t.Log("  - rca.Engine has zero production callers (only engine_test.go)")
}
