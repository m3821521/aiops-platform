package anomaly_test

import (
	"context"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/prometheus/common/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建内存 SQLite 数据库并迁移表。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&anomaly.AnomalyRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRepository_CreateAndFind(t *testing.T) {
	db := setupTestDB(t)
	repo := anomaly.NewRepository(db)
	ctx := context.Background()

	rec := &anomaly.AnomalyRecord{
		Metric:       "cpu_usage",
		ResourceType: "node",
		ResourceName: "node1",
		Namespace:    "default",
		Cluster:      "local",
		Timestamp:    time.Now(),
		Value:        95.5,
		Baseline:     50.0,
		AnomalyScore: 0.9,
		Severity:     "critical",
		Algorithm:    "static_threshold",
		Reason:       "CPU 超过阈值",
		Status:       "active",
	}

	saved, err := repo.Create(ctx, rec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	found, err := repo.FindByID(ctx, saved.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Metric != "cpu_usage" {
		t.Fatalf("expected metric cpu_usage, got %s", found.Metric)
	}
}

func TestRepository_Upsert_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	repo := anomaly.NewRepository(db)
	ctx := context.Background()
	ts := time.Now().Truncate(time.Second)

	rec := &anomaly.AnomalyRecord{
		Metric:       "cpu_usage",
		ResourceType: "node",
		ResourceName: "node1",
		Cluster:      "local",
		Timestamp:    ts,
		Value:        90.0,
		AnomalyScore: 0.8,
		Severity:     "critical",
		Algorithm:    "static_threshold",
		Status:       "active",
	}

	// 第一次创建。
	first, isNew, err := repo.Upsert(ctx, rec)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if !isNew {
		t.Fatal("expected isNew=true on first upsert")
	}

	// 第二次相同标识，应该更新而不是创建。
	rec.Value = 95.0
	second, isNew2, err := repo.Upsert(ctx, rec)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if isNew2 {
		t.Fatal("expected isNew=false on second upsert")
	}
	if second.ID != first.ID {
		t.Fatalf("expected same ID %d, got %d", first.ID, second.ID)
	}
	if second.Value != 95.0 {
		t.Fatalf("expected updated value 95.0, got %f", second.Value)
	}

	// 总数应该是 1。
	records, total, err := repo.List(ctx, anomaly.ListFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestRepository_ListFilters(t *testing.T) {
	db := setupTestDB(t)
	repo := anomaly.NewRepository(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		severity := "warning"
		if i%2 == 0 {
			severity = "critical"
		}
		_, _ = repo.Create(ctx, &anomaly.AnomalyRecord{
			Metric:       "metric_" + string(rune('a'+i)),
			ResourceType: "pod",
			ResourceName: "pod-" + string(rune('a'+i)),
			Namespace:    "ns-" + string(rune('a'+i%2)),
			Cluster:      "local",
			Timestamp:    time.Now().Add(time.Duration(i) * time.Minute),
			Value:        float64(80 + i),
			AnomalyScore: 0.5 + float64(i)*0.1,
			Severity:     severity,
			Algorithm:    "static_threshold",
			Status:       "active",
		})
	}

	// 按 severity 筛选。
	records, total, err := repo.List(ctx, anomaly.ListFilter{Severity: "critical"}, 1, 10)
	if err != nil {
		t.Fatalf("list by severity: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 critical, got %d", total)
	}
	_ = records

	// 按 namespace 筛选。
	_, total, err = repo.List(ctx, anomaly.ListFilter{Namespace: "ns-a"}, 1, 10)
	if err != nil {
		t.Fatalf("list by namespace: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 in ns-a, got %d", total)
	}
}

func TestRepository_FindActiveAndCount(t *testing.T) {
	db := setupTestDB(t)
	repo := anomaly.NewRepository(db)
	ctx := context.Background()

	// 2 active, 1 resolved。
	_, _ = repo.Create(ctx, &anomaly.AnomalyRecord{Metric: "m1", Timestamp: time.Now(), Status: "active", Severity: "warning", Algorithm: "static_threshold"})
	_, _ = repo.Create(ctx, &anomaly.AnomalyRecord{Metric: "m2", Timestamp: time.Now(), Status: "active", Severity: "critical", Algorithm: "static_threshold"})
	resolved, _ := repo.Create(ctx, &anomaly.AnomalyRecord{Metric: "m3", Timestamp: time.Now(), Status: "resolved", Severity: "warning", Algorithm: "static_threshold"})

	count, err := repo.CountActive(ctx)
	if err != nil {
		t.Fatalf("count active: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 active, got %d", count)
	}

	// 更新为 resolved。
	if err := repo.UpdateStatus(ctx, resolved.ID, "resolved"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	active, err := repo.FindActive(ctx, anomaly.ListFilter{})
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active records, got %d", len(active))
	}
}

func TestService_DetectAndPersist(t *testing.T) {
	db := setupTestDB(t)
	repo := anomaly.NewRepository(db)

	now := time.Now()
	matrix := model.Matrix{
		{
			Metric: model.Metric{"__name__": "cpu_usage", "pod": "test-pod", "namespace": "default"},
			Values: []model.SamplePair{
				{Timestamp: model.Time(now.Unix()), Value: 50},
				{Timestamp: model.Time(now.Add(time.Minute).Unix()), Value: 85},
				{Timestamp: model.Time(now.Add(2 * time.Minute).Unix()), Value: 95},
			},
		},
	}

	querier := &mockQuerier{matrix: matrix}
	svc := anomaly.NewServiceWithRepo(querier, repo)

	warning := 80.0
	critical := 90.0
	result, err := svc.DetectAndPersist(context.Background(), anomaly.DetectRequest{
		Query:        "cpu_usage",
		Start:        now,
		End:          now.Add(2 * time.Minute),
		Step:         time.Minute,
		ResourceType: "pod",
		Cluster:      "local",
		Thresholds: anomaly.ThresholdConfig{
			UpperWarning:  &warning,
			UpperCritical: &critical,
		},
	})
	if err != nil {
		t.Fatalf("detect and persist: %v", err)
	}
	if result.SavedCount != 1 {
		t.Fatalf("expected saved 1 (deduplicated), got %d", result.SavedCount)
	}

	// 验证数据库中有 1 条记录（去重合并：同一资源+metric只保留最新一条）。
	records, total, err := repo.List(context.Background(), anomaly.ListFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 record (deduplicated), got %d", total)
	}
	if records[0].ResourceName != "test-pod" {
		t.Fatalf("expected resource test-pod, got %s", records[0].ResourceName)
	}
	// 验证取的是最新异常点（value=95, severity=critical）。
	if records[0].Value != 95 {
		t.Fatalf("expected latest value 95, got %v", records[0].Value)
	}
	if records[0].Severity != "critical" {
		t.Fatalf("expected severity critical, got %s", records[0].Severity)
	}
	if records[0].Status != "active" {
		t.Fatalf("expected status active, got %s", records[0].Status)
	}
}

// mockIncidentSink 实现 anomaly.IncidentSink 接口。
type mockIncidentSink struct {
	called    bool
	signals   []anomaly.AnomalySignal
	returnID  int64
	returnErr error
}

func (m *mockIncidentSink) IngestAnomalySignal(_ context.Context, sig anomaly.AnomalySignal) (int64, error) {
	m.called = true
	m.signals = append(m.signals, sig)
	return m.returnID, m.returnErr
}

func TestService_DetectAndPersist_WithIncidentSink(t *testing.T) {
	db := setupTestDB(t)
	repo := anomaly.NewRepository(db)

	now := time.Now()
	matrix := model.Matrix{
		{
			Metric: model.Metric{"__name__": "cpu_usage", "pod": "test-pod", "namespace": "default"},
			Values: []model.SamplePair{
				{Timestamp: model.Time(now.Unix()), Value: 95},
			},
		},
	}

	querier := &mockQuerier{matrix: matrix}
	svc := anomaly.NewServiceWithRepo(querier, repo)
	sink := &mockIncidentSink{returnID: 42}
	svc.SetIncidentSink(sink)

	warning := 80.0
	critical := 90.0
	_, err := svc.DetectAndPersist(context.Background(), anomaly.DetectRequest{
		Query:        "cpu_usage",
		Start:        now,
		End:          now.Add(time.Minute),
		Step:         time.Minute,
		ResourceType: "pod",
		Cluster:      "local",
		Thresholds: anomaly.ThresholdConfig{
			UpperWarning:  &warning,
			UpperCritical: &critical,
		},
	})
	if err != nil {
		t.Fatalf("detect and persist: %v", err)
	}

	if !sink.called {
		t.Fatal("expected incident sink to be called")
	}
	if len(sink.signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sink.signals))
	}
	if sink.signals[0].ResourceName != "test-pod" {
		t.Fatalf("expected resource test-pod, got %s", sink.signals[0].ResourceName)
	}
	if sink.signals[0].Metric != "cpu_usage" {
		t.Fatalf("expected metric cpu_usage, got %s", sink.signals[0].Metric)
	}

	// 验证 anomaly 记录关联了 incident_id。
	records, _, _ := repo.List(context.Background(), anomaly.ListFilter{}, 1, 10)
	if records[0].IncidentID == nil || *records[0].IncidentID != 42 {
		t.Fatalf("expected incident_id 42, got %v", records[0].IncidentID)
	}
}

func TestService_DetectAndPersist_PrometheusUnavailable(t *testing.T) {
	// querier 为 nil 时应该返回明确错误。
	svc := anomaly.NewServiceWithRepo(nil, nil)
	_, err := svc.DetectAndPersist(context.Background(), anomaly.DetectRequest{
		Query: "cpu",
		Start: time.Now(),
		End:   time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("expected error when querier is nil")
	}
}

func TestDefaultRules(t *testing.T) {
	rules := anomaly.DefaultRules()
	if len(rules) != 4 {
		t.Fatalf("expected 4 default rules, got %d", len(rules))
	}
	for _, r := range rules {
		if !r.Enabled {
			t.Fatalf("rule %s should be enabled", r.Name)
		}
		if r.Interval <= 0 {
			t.Fatalf("rule %s should have positive interval", r.Name)
		}
		if r.Metric == "" {
			t.Fatalf("rule %s should have metric", r.Name)
		}
	}
}

// 确保 mockQuerier 实现 monitoring.Querier 接口。
var _ monitoring.Querier = (*mockQuerier)(nil)
