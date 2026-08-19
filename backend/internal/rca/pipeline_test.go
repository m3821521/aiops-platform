package rca

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

// Scenario 1: Node Memory Pressure → Pod OOMKilled → Pod Restart → Service Alert
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

	if result.Status != RCAStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	if len(result.Candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}

	// Pod 应该是 Top Candidate（有 OOMKilled 强证据）。
	top := result.Candidates[0]
	if top.ResourceType != "pod" || top.ResourceName != "order-service-abc" {
		t.Errorf("expected top candidate pod/order-service-abc, got %s/%s", top.ResourceType, top.ResourceName)
	}
	if result.Confidence <= 0 {
		t.Error("expected confidence > 0")
	}
	t.Logf("root cause: %s, confidence: %.2f", result.RootCause, result.Confidence)
}

// Scenario 2: Pod CrashLoopBackOff → Pod Restart → Service Alert
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

	if result.Status != RCAStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	top := result.Candidates[0]
	if top.ResourceType != "pod" {
		t.Errorf("expected top candidate pod, got %s", top.ResourceType)
	}
	t.Logf("root cause: %s, confidence: %.2f", result.RootCause, result.Confidence)
}

// Scenario 3: ImagePullBackOff → Pod Pending
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

	if result.Status != RCAStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	top := result.Candidates[0]
	if top.ResourceType != "pod" {
		t.Errorf("expected top candidate pod, got %s", top.ResourceType)
	}
	t.Logf("root cause: %s, confidence: %.2f", result.RootCause, result.Confidence)
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
