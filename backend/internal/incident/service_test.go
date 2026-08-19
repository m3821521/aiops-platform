package incident

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Incident{}, &IncidentSignal{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func newTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	repo := NewRepository(db)
	correlator := NewCorrelator(DefaultCorrelationConfig())
	return NewService(repo, correlator), db
}

func makeAlertSignal(fingerprint, alertname, service, namespace, pod string, resolved bool) Signal {
	return Signal{
		SignalType:   SignalAlert,
		SignalID:     fingerprint,
		Title:        alertname,
		Severity:     SeverityWarning,
		Cluster:      "local",
		Namespace:    namespace,
		Service:      service,
		ResourceType: ResourcePod,
		ResourceName: pod,
		Timestamp:    time.Now(),
		Resolved:     resolved,
		Labels:       map[string]string{"app": service},
	}
}

// 场景1: Firing Alert → Incident Created
func TestIngestSignal_CreatesIncident(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig := makeAlertSignal("fp-001", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc, _, err := svc.IngestSignal(ctx, sig)
	if err != nil {
		t.Fatalf("IngestSignal failed: %v", err)
	}
	if inc.ID == 0 {
		t.Fatal("expected incident ID > 0")
	}
	if inc.Title != "HighCPU" {
		t.Errorf("expected title HighCPU, got %s", inc.Title)
	}
	if inc.Status != StatusOpen {
		t.Errorf("expected status open, got %s", inc.Status)
	}
	if inc.SignalCount != 1 {
		t.Errorf("expected signal_count 1, got %d", inc.SignalCount)
	}
}

// 场景2: 相同 fingerprint Alert → 不重复 Incident
func TestIngestSignal_Deduplication(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig := makeAlertSignal("fp-002", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc1, _, err := svc.IngestSignal(ctx, sig)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}

	// 相同 fingerprint 再次接入
	inc2, _, err := svc.IngestSignal(ctx, sig)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if inc1.ID != inc2.ID {
		t.Errorf("expected same incident ID, got %d and %d", inc1.ID, inc2.ID)
	}

	// 验证只有 1 个 Incident
	incidents, total, _ := svc.List(ctx, ListFilter{}, 1, 10)
	if total != 1 {
		t.Errorf("expected 1 incident, got %d", total)
	}
	if len(incidents) != 1 {
		t.Errorf("expected 1 incident in list, got %d", len(incidents))
	}
}

// 场景3: 同 Service 多个 Alert → 关联同一个 Incident
func TestIngestSignal_SameServiceCorrelation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig1 := makeAlertSignal("fp-003", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc1, _, err := svc.IngestSignal(ctx, sig1)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}

	// 同 service 不同 pod 的告警
	sig2 := makeAlertSignal("fp-004", "HighMemory", "order-service", "default", "order-pod-2", false)
	inc2, _, err := svc.IngestSignal(ctx, sig2)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}

	if inc1.ID != inc2.ID {
		t.Errorf("expected same incident for same service, got %d and %d", inc1.ID, inc2.ID)
	}
	if inc2.SignalCount != 2 {
		t.Errorf("expected signal_count 2, got %d", inc2.SignalCount)
	}
}

// 场景4: 不同 Service → 创建不同 Incident
func TestIngestSignal_DifferentService(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig1 := makeAlertSignal("fp-005", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc1, _, _ := svc.IngestSignal(ctx, sig1)

	sig2 := makeAlertSignal("fp-006", "HighCPU", "payment-service", "default", "payment-pod-1", false)
	inc2, _, _ := svc.IngestSignal(ctx, sig2)

	if inc1.ID == inc2.ID {
		t.Error("expected different incidents for different services")
	}

	_, total, _ := svc.List(ctx, ListFilter{}, 1, 10)
	if total != 2 {
		t.Errorf("expected 2 incidents, got %d", total)
	}
}

// 场景5: Alert Resolved → Signal Resolved
func TestIngestSignal_ResolvedUpdatesSignal(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// 先 firing
	sig := makeAlertSignal("fp-007", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc, _, _ := svc.IngestSignal(ctx, sig)

	// 再 resolved
	sigResolved := makeAlertSignal("fp-007", "HighCPU", "order-service", "default", "order-pod-1", true)
	inc, _, err := svc.IngestSignal(ctx, sigResolved)
	if err != nil {
		t.Fatalf("resolved ingest failed: %v", err)
	}

	// 验证信号状态
	signals, _ := svc.repo.ListSignals(ctx, inc.ID)
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if !signals[0].Resolved {
		t.Error("expected signal to be resolved")
	}
	if signals[0].ResolvedAt == nil {
		t.Error("expected resolved_at to be set")
	}
}

// 场景6: 所有主要 Signal Resolved → Incident Resolved
func TestIngestSignal_AllResolvedAutoResolvesIncident(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig1 := makeAlertSignal("fp-008", "HighCPU", "order-service", "default", "order-pod-1", false)
	sig2 := makeAlertSignal("fp-009", "HighMemory", "order-service", "default", "order-pod-2", false)
	inc, _, _ := svc.IngestSignal(ctx, sig1)
	svc.IngestSignal(ctx, sig2)

	// 两个都 resolved
	sig1r := makeAlertSignal("fp-008", "HighCPU", "order-service", "default", "order-pod-1", true)
	sig2r := makeAlertSignal("fp-009", "HighMemory", "order-service", "default", "order-pod-2", true)
	svc.IngestSignal(ctx, sig1r)
	inc, _, _ = svc.IngestSignal(ctx, sig2r)

	if inc.Status != StatusResolved {
		t.Errorf("expected incident resolved, got %s", inc.Status)
	}
	if inc.EndTime == nil {
		t.Error("expected end_time to be set")
	}
}

// 场景7: Incident Acknowledge → 状态变化
func TestAcknowledge(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig := makeAlertSignal("fp-010", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc, _, _ := svc.IngestSignal(ctx, sig)

	if err := svc.Acknowledge(ctx, inc.ID); err != nil {
		t.Fatalf("acknowledge failed: %v", err)
	}

	inc, _ = svc.Get(ctx, inc.ID)
	if inc.Status != StatusAcknowledged {
		t.Errorf("expected status acknowledged, got %s", inc.Status)
	}
}

// 场景8: Incident Close → 状态变化
func TestClose(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig := makeAlertSignal("fp-011", "HighCPU", "order-service", "default", "order-pod-1", false)
	inc, _, _ := svc.IngestSignal(ctx, sig)

	if err := svc.Close(ctx, inc.ID); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	inc, _ = svc.Get(ctx, inc.ID)
	if inc.Status != StatusClosed {
		t.Errorf("expected status closed, got %s", inc.Status)
	}
}

// 关联评分测试
func TestCorrelationScore(t *testing.T) {
	cfg := DefaultCorrelationConfig()
	c := NewCorrelator(cfg)

	inc := &Incident{
		Title:        "test",
		Service:      "order-service",
		Namespace:    "default",
		ResourceType: "pod",
		ResourceName: "order-pod-1",
		StartTime:    time.Now(),
	}

	// 同 pod 同 namespace → 高分
	sig := Signal{
		SignalType:   SignalAlert,
		SignalID:     "fp-1",
		Namespace:    "default",
		Service:      "order-service",
		ResourceType: ResourcePod,
		ResourceName: "order-pod-1",
		Timestamp:    time.Now(),
	}
	score := c.Score(sig, inc)
	if score.Total < cfg.ScoreThreshold {
		t.Errorf("expected score >= threshold for same pod, got %.2f", score.Total)
	}

	// 不同 service 不同 pod → 低分
	sig2 := Signal{
		SignalType:   SignalAlert,
		SignalID:     "fp-2",
		Namespace:    "other-ns",
		Service:      "payment-service",
		ResourceType: ResourcePod,
		ResourceName: "payment-pod-1",
		Timestamp:    time.Now().Add(-time.Hour), // 超出时间窗口
	}
	score2 := c.Score(sig2, inc)
	if score2.Total >= cfg.ScoreThreshold {
		t.Errorf("expected score < threshold for unrelated signal, got %.2f", score2.Total)
	}
}

// Timeline 测试
func TestGetTimeline(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sig1 := makeAlertSignal("fp-012", "HighCPU", "order-service", "default", "order-pod-1", false)
	sig2 := makeAlertSignal("fp-013", "HighMemory", "order-service", "default", "order-pod-2", false)
	inc, _, _ := svc.IngestSignal(ctx, sig1)
	svc.IngestSignal(ctx, sig2)

	timeline, err := svc.GetTimeline(ctx, inc.ID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 2 {
		t.Errorf("expected 2 timeline events, got %d", len(timeline))
	}
}
