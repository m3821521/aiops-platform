package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aiops/aiops-platform/internal/api"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/handler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHealthAndNodes(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	mgr := cluster.NewManager(nil)
	mgr.SetClient("demo", client)

	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(mgr)},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health status=%d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?cluster=demo", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("nodes status=%d body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"].(float64) != 0 {
		t.Fatalf("unexpected body=%v", body)
	}
}

func TestHealthOKWithoutDB(t *testing.T) {
	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	r := api.NewRouter("test", api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
	})

	// 先访问一次 /health，让 metrics 中间件产生数据。
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	// 验证 /metrics 返回 200 且包含 Prometheus 文本格式。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
		t.Fatalf("metrics body does not look like prometheus format, head=%s", body[:200])
	}
	// 自定义 HTTP 指标应该已注册。
	if !strings.Contains(body, "aiops_http_requests_total") {
		t.Fatalf("expected aiops_http_requests_total in metrics, head=%s", body[:200])
	}
}
