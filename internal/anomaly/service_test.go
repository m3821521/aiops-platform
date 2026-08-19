package anomaly_test

import (
	"context"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/prometheus/common/model"
)

// mockQuerier 实现 monitoring.Querier 接口，用于测试。
type mockQuerier struct {
	matrix model.Matrix
}

func (m *mockQuerier) Query(_ context.Context, _ string, _ time.Time) (*monitoring.QueryResult, error) {
	return nil, nil
}

func (m *mockQuerier) QueryRange(_ context.Context, _ string, _, _ time.Time, _ time.Duration) (*monitoring.QueryResult, error) {
	return &monitoring.QueryResult{
		ResultType: "matrix",
		Result:     m.matrix,
	}, nil
}

func (m *mockQuerier) NodeMetrics(_ context.Context) (*monitoring.NodeMetrics, error) {
	return nil, nil
}

func (m *mockQuerier) PodMetrics(_ context.Context, _ string) (*monitoring.PodMetrics, error) {
	return nil, nil
}

func TestAnomalyServiceDetect(t *testing.T) {
	now := time.Now()
	matrix := model.Matrix{
		{
			Metric: model.Metric{"__name__": "cpu_usage"},
			Values: []model.SamplePair{
				{Timestamp: model.Time(now.Unix()), Value: 50},
				{Timestamp: model.Time(now.Add(time.Minute).Unix()), Value: 85},
				{Timestamp: model.Time(now.Add(2 * time.Minute).Unix()), Value: 95},
			},
		},
	}

	svc := anomaly.NewService(&mockQuerier{matrix: matrix})

	warning := 80.0
	critical := 90.0
	result, err := svc.Detect(context.Background(), anomaly.DetectRequest{
		Query: "cpu_usage",
		Start: now,
		End:   now.Add(2 * time.Minute),
		Step:  time.Minute,
		Thresholds: anomaly.ThresholdConfig{
			UpperWarning:  &warning,
			UpperCritical: &critical,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 50 正常，85 warning，95 critical → 2 个异常。
	if len(result.Anomalies) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(result.Anomalies))
	}
	if result.Metric != "cpu_usage" {
		t.Fatalf("expected metric cpu_usage, got %s", result.Metric)
	}
}

func TestAnomalyServiceInvalidRequest(t *testing.T) {
	svc := anomaly.NewService(&mockQuerier{})

	// 空 query。
	_, err := svc.Detect(context.Background(), anomaly.DetectRequest{
		Start: time.Now(),
		End:   time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("expected error for empty query")
	}

	// end 早于 start。
	_, err = svc.Detect(context.Background(), anomaly.DetectRequest{
		Query: "cpu",
		Start: time.Now().Add(time.Hour),
		End:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for end before start")
	}
}
