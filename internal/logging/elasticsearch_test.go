package logging_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/logging"
)

func TestParseSearchResponse(t *testing.T) {
	// 模拟 Elasticsearch 响应。
	resp := `{
		"took": 5,
		"hits": {
			"total": {"value": 2},
			"hits": [
				{
					"_index": "filebeat-2024.01.01",
					"_id": "abc123",
					"_score": 1.0,
					"_source": {
						"@timestamp": "2024-01-01T10:00:00Z",
						"message": "Connection refused",
						"log": {"level": "error"},
						"kubernetes": {
							"namespace": "default",
							"pod": {"name": "order-service-abc"},
							"container": {"name": "order-service"}
						}
					}
				},
				{
					"_index": "filebeat-2024.01.01",
					"_id": "def456",
					"_score": 0.5,
					"_source": {
						"@timestamp": "2024-01-01T10:01:00Z",
						"message": "Request timeout",
						"level": "warn",
						"namespace": "kube-system",
						"pod": "kube-proxy-xyz",
						"container": "kube-proxy"
					}
				}
			]
		}
	}`

	// 通过 mock server 测试完整流程。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer server.Close()

	client := logging.NewClient(server.URL, "filebeat-*", "", "", 10)
	result, err := client.Search(context.Background(), logging.SearchQuery{
		Keyword: "error",
		Size:    10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 2 {
		t.Fatalf("expected total 2, got %d", result.Total)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.Hits[0].Message != "Connection refused" {
		t.Fatalf("expected message 'Connection refused', got %s", result.Hits[0].Message)
	}
	if result.Hits[0].Level != "error" {
		t.Fatalf("expected level 'error', got %s", result.Hits[0].Level)
	}
	if result.Hits[0].Namespace != "default" {
		t.Fatalf("expected namespace 'default', got %s", result.Hits[0].Namespace)
	}
	if result.Hits[0].Pod != "order-service-abc" {
		t.Fatalf("expected pod 'order-service-abc', got %s", result.Hits[0].Pod)
	}
	// 第二条用扁平字段。
	if result.Hits[1].Level != "warn" {
		t.Fatalf("expected level 'warn', got %s", result.Hits[1].Level)
	}
	if result.Hits[1].Namespace != "kube-system" {
		t.Fatalf("expected namespace 'kube-system', got %s", result.Hits[1].Namespace)
	}
}

func TestSearchQueryDSL(t *testing.T) {
	// 验证查询 DSL 构建正确。
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"took":0,"hits":{"total":{"value":0},"hits":[]}}`))
	}))
	defer server.Close()

	client := logging.NewClient(server.URL, "test-*", "", "", 10)
	now := time.Now()
	_, err := client.Search(context.Background(), logging.SearchQuery{
		Keyword:   "error",
		Namespace: "default",
		Pod:       "my-pod",
		Level:     "error",
		Start:     now.Add(-1 * time.Hour),
		End:       now,
		Size:      50,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证 query.bool.must 有 match。
	query := receivedBody["query"].(map[string]interface{})
	boolQ := query["bool"].(map[string]interface{})
	must := boolQ["must"].([]interface{})
	if len(must) != 1 {
		t.Fatalf("expected 1 must clause, got %d", len(must))
	}

	// 验证 filter 有 namespace、pod、level、time range 共 4 个。
	filter := boolQ["filter"].([]interface{})
	if len(filter) != 4 {
		t.Fatalf("expected 4 filter clauses, got %d", len(filter))
	}

	// 验证 size。
	if receivedBody["size"].(float64) != 50 {
		t.Fatalf("expected size 50, got %v", receivedBody["size"])
	}
}

func TestSearchDefaultSize(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"took":0,"hits":{"total":{"value":0},"hits":[]}}`))
	}))
	defer server.Close()

	client := logging.NewClient(server.URL, "test-*", "", "", 10)
	_, _ = client.Search(context.Background(), logging.SearchQuery{})

	if receivedBody["size"].(float64) != 100 {
		t.Fatalf("expected default size 100, got %v", receivedBody["size"])
	}
}

func TestSearchESUnavailable(t *testing.T) {
	// 指向一个不存在的地址。
	client := logging.NewClient("http://127.0.0.1:19999", "test-*", "", "", 1)
	_, err := client.Search(context.Background(), logging.SearchQuery{Keyword: "test"})
	if err == nil {
		t.Fatal("expected error for unavailable ES")
	}
}

func TestSearchNilClient(t *testing.T) {
	var client *logging.Client
	_, err := client.Search(context.Background(), logging.SearchQuery{})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestSearchBasicAuth(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Write([]byte(`{"took":0,"hits":{"total":{"value":0},"hits":[]}}`))
	}))
	defer server.Close()

	client := logging.NewClient(server.URL, "test-*", "user", "pass", 10)
	_, _ = client.Search(context.Background(), logging.SearchQuery{})

	if authHeader == "" {
		t.Fatal("expected Authorization header")
	}
}
