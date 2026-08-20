package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/api"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/gin-gonic/gin"
)

// newMockPrometheus 启动一个模拟 Prometheus API 的 HTTP 服务器，所有路径返回相同响应。
func newMockPrometheus(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

// vectorResp 构造一个 vector 类型的成功响应。
func vectorResp() string {
	return `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1700000000,"1"]}]}}`
}

// matrixResp 构造一个 matrix 类型的成功响应。
func matrixResp() string {
	return `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up"},"values":[[1700000000,"1"],[1700000015,"1"]]}]}}`
}

// newTestRouter 创建一个带 mock Prometheus 的测试路由。
func newTestRouter(prom *monitoring.Client) *gin.Engine {
	return api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
		Metrics: &handler.MetricsHandler{Prom: prom},
	})
}

func TestMetricsQuerySuccess(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusOK, `{
		"status": "success",
		"data": {
			"resultType": "vector",
			"result": [
				{"metric": {"__name__": "up", "job": "prometheus"}, "value": [1700000000, "1"]}
			]
		}
	}`)
	defer mock.Close()

	prom, err := monitoring.NewClient(mock.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
		Metrics: &handler.MetricsHandler{Prom: prom},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/query?query=up", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 0 {
		t.Fatalf("unexpected code=%d body=%s", resp.Code, w.Body.String())
	}
	if !strings.Contains(string(resp.Data), "resultType") || !strings.Contains(string(resp.Data), "vector") {
		t.Fatalf("unexpected data: %s", resp.Data)
	}
}

func TestMetricsQueryMissingQuery(t *testing.T) {
	prom, _ := monitoring.NewClient("http://127.0.0.1:9090", 5*time.Second)
	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
		Metrics: &handler.MetricsHandler{Prom: prom},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/query", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsQueryInvalidTime(t *testing.T) {
	prom, _ := monitoring.NewClient("http://127.0.0.1:9090", 5*time.Second)
	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
		Metrics: &handler.MetricsHandler{Prom: prom},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/query?query=up&time=not-a-time", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsQueryPrometheusError(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusBadRequest, `{
		"status": "error",
		"errorType": "bad_data",
		"error": "invalid query expression"
	}`)
	defer mock.Close()

	prom, _ := monitoring.NewClient(mock.URL, 5*time.Second)
	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
		Metrics: &handler.MetricsHandler{Prom: prom},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/query?query=bad(", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsRangeSuccess(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusOK, matrixResp())
	defer mock.Close()

	prom, _ := monitoring.NewClient(mock.URL, 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/metrics/range?query=up&start=2026-01-01T00:00:00Z&end=2026-01-01T00:05:00Z&step=15s", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "matrix") {
		t.Fatalf("expected matrix result, body=%s", w.Body.String())
	}
}

func TestMetricsRangeMissingStart(t *testing.T) {
	prom, _ := monitoring.NewClient("http://127.0.0.1:9090", 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/metrics/range?query=up&end=2026-01-01T00:05:00Z&step=15s", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsRangeInvalidStep(t *testing.T) {
	prom, _ := monitoring.NewClient("http://127.0.0.1:9090", 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/metrics/range?query=up&start=2026-01-01T00:00:00Z&end=2026-01-01T00:05:00Z&step=abc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsNodesSuccess(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusOK, vectorResp())
	defer mock.Close()

	prom, _ := monitoring.NewClient(mock.URL, 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/nodes", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	// NodeMetrics 包含 cpu 和 memory 两个字段。
	if !strings.Contains(w.Body.String(), `"cpu"`) || !strings.Contains(w.Body.String(), `"memory"`) {
		t.Fatalf("expected cpu/memory fields, body=%s", w.Body.String())
	}
}

func TestMetricsPodsSuccess(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusOK, vectorResp())
	defer mock.Close()

	prom, _ := monitoring.NewClient(mock.URL, 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/pods?namespace=default", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsPodsInvalidNamespace(t *testing.T) {
	prom, _ := monitoring.NewClient("http://127.0.0.1:9090", 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/pods?namespace=Invalid_Name!", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsPodsEmptyNamespace(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusOK, vectorResp())
	defer mock.Close()

	prom, _ := monitoring.NewClient(mock.URL, 5*time.Second)
	r := newTestRouter(prom)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/pods", nil)
	r.ServeHTTP(w, req)

	// 空 namespace 应该查询所有命名空间，返回 200。
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty namespace, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestMetricsPodsMalformedNamespace(t *testing.T) {
	prom, _ := monitoring.NewClient("http://127.0.0.1:9090", 5*time.Second)
	r := newTestRouter(prom)

	cases := []string{
		"Invalid_Name",
		"-start-with-dash",
		"end-with-dash-",
		"special@char",
		"UPPERCASE",
		"has%20space", // URL 编码的空格
	}

	for _, ns := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/pods?namespace="+ns, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for namespace=%q, got %d body=%s", ns, w.Code, w.Body.String())
		}
	}
}

func TestMetricsPodsValidNamespace(t *testing.T) {
	mock := newMockPrometheus(t, http.StatusOK, vectorResp())
	defer mock.Close()

	prom, _ := monitoring.NewClient(mock.URL, 5*time.Second)
	r := newTestRouter(prom)

	cases := []string{
		"default",
		"kube-system",
		"production",
		"ns-123",
		"a",
		"abc123",
	}

	for _, ns := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/pods?namespace="+ns, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for valid namespace=%q, got %d body=%s", ns, w.Code, w.Body.String())
		}
	}
}
