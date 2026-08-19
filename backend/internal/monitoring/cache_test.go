package monitoring_test

import (
	"context"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// mockQuerier 记录调用次数，用于验证缓存是否命中。
type mockQuerier struct {
	calls int
}

func (m *mockQuerier) Query(_ context.Context, _ string, _ time.Time) (*monitoring.QueryResult, error) {
	m.calls++
	return &monitoring.QueryResult{ResultType: "vector", Result: []any{}}, nil
}

func (m *mockQuerier) QueryRange(_ context.Context, _ string, _, _ time.Time, _ time.Duration) (*monitoring.QueryResult, error) {
	m.calls++
	return &monitoring.QueryResult{ResultType: "matrix", Result: []any{}}, nil
}

func (m *mockQuerier) NodeMetrics(_ context.Context) (*monitoring.NodeMetrics, error) {
	m.calls++
	return &monitoring.NodeMetrics{}, nil
}

func (m *mockQuerier) PodMetrics(_ context.Context, _ string) (*monitoring.PodMetrics, error) {
	m.calls++
	return &monitoring.PodMetrics{}, nil
}

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestCachedQueryHit(t *testing.T) {
	rdb := newTestRedis(t)
	mock := &mockQuerier{}
	cached := monitoring.NewCachedQuerier(mock, rdb)

	ctx := context.Background()
	ts := time.Unix(1700000000, 0)

	// 第一次：未命中，调用 inner。
	if _, err := cached.Query(ctx, "up", ts); err != nil {
		t.Fatal(err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 call, got %d", mock.calls)
	}

	// 第二次：相同参数，命中缓存，不调用 inner。
	if _, err := cached.Query(ctx, "up", ts); err != nil {
		t.Fatal(err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected cache hit (still 1 call), got %d", mock.calls)
	}
}

func TestCachedQueryDifferentTime(t *testing.T) {
	rdb := newTestRedis(t)
	mock := &mockQuerier{}
	cached := monitoring.NewCachedQuerier(mock, rdb)

	ctx := context.Background()

	cached.Query(ctx, "up", time.Unix(1700000000, 0))
	cached.Query(ctx, "up", time.Unix(1700000015, 0))

	if mock.calls != 2 {
		t.Fatalf("different timestamps should miss cache, expected 2 calls, got %d", mock.calls)
	}
}

func TestCachedQueryRangeHit(t *testing.T) {
	rdb := newTestRedis(t)
	mock := &mockQuerier{}
	cached := monitoring.NewCachedQuerier(mock, rdb)

	ctx := context.Background()
	start := time.Unix(1700000000, 0)
	end := time.Unix(1700000300, 0)

	cached.QueryRange(ctx, "up", start, end, 15*time.Second)
	cached.QueryRange(ctx, "up", start, end, 15*time.Second)

	if mock.calls != 1 {
		t.Fatalf("expected cache hit (1 call), got %d", mock.calls)
	}
}

func TestCachedNilRedis(t *testing.T) {
	mock := &mockQuerier{}
	// rdb 为 nil 时，NewCachedQuerier 直接返回 inner，不包装。
	cached := monitoring.NewCachedQuerier(mock, nil)

	ctx := context.Background()
	cached.Query(ctx, "up", time.Time{})
	cached.Query(ctx, "up", time.Time{})

	// 无缓存，每次都调用 inner。
	if mock.calls != 2 {
		t.Fatalf("no cache should call inner every time, expected 2 calls, got %d", mock.calls)
	}
}

func TestCachedNodeMetricsHit(t *testing.T) {
	rdb := newTestRedis(t)
	mock := &mockQuerier{}
	cached := monitoring.NewCachedQuerier(mock, rdb)

	ctx := context.Background()
	cached.NodeMetrics(ctx)
	cached.NodeMetrics(ctx)

	if mock.calls != 1 {
		t.Fatalf("expected cache hit (1 call), got %d", mock.calls)
	}
}

func TestCachedPodMetricsDifferentNamespace(t *testing.T) {
	rdb := newTestRedis(t)
	mock := &mockQuerier{}
	cached := monitoring.NewCachedQuerier(mock, rdb)

	ctx := context.Background()
	cached.PodMetrics(ctx, "default")
	cached.PodMetrics(ctx, "kube-system")

	if mock.calls != 2 {
		t.Fatalf("different namespaces should miss cache, expected 2 calls, got %d", mock.calls)
	}
}
