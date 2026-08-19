package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aiops/aiops-platform/internal/ai"
)

func TestOpenAIProviderChat(t *testing.T) {
	var receivedReq map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [
				{"message": {"role": "assistant", "content": "这是 AI 的回复"}}
			]
		}`))
	}))
	defer server.Close()

	provider := ai.NewOpenAIProvider(server.URL, "test-key", "test-model", 10)
	reply, err := provider.Chat(context.Background(), []ai.Message{
		{Role: "user", Content: "你好"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "这是 AI 的回复" {
		t.Fatalf("unexpected reply: %s", reply)
	}

	// 验证请求体。
	if receivedReq["model"] != "test-model" {
		t.Fatalf("expected model test-model, got %v", receivedReq["model"])
	}
	msgs := receivedReq["messages"].([]interface{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestOpenAIProviderAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	provider := ai.NewOpenAIProvider(server.URL, "secret-key", "model", 10)
	_, _ = provider.Chat(context.Background(), []ai.Message{{Role: "user", Content: "hi"}})

	if authHeader != "Bearer secret-key" {
		t.Fatalf("expected Bearer secret-key, got %s", authHeader)
	}
}

func TestOpenAIProviderNoAuth(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	// 空 apiKey，不应设置 Authorization 头。
	provider := ai.NewOpenAIProvider(server.URL, "", "model", 10)
	_, _ = provider.Chat(context.Background(), []ai.Message{{Role: "user", Content: "hi"}})

	if authHeader != "" {
		t.Fatalf("expected no auth header, got %s", authHeader)
	}
}

func TestOpenAIProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error": {"message": "Invalid API key"}}`))
	}))
	defer server.Close()

	provider := ai.NewOpenAIProvider(server.URL, "bad-key", "model", 10)
	_, err := provider.Chat(context.Background(), []ai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestOpenAIProviderEmptyMessages(t *testing.T) {
	provider := ai.NewOpenAIProvider("http://localhost", "key", "model", 10)
	_, err := provider.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestOpenAIProviderNil(t *testing.T) {
	var provider *ai.OpenAIProvider
	_, err := provider.Chat(context.Background(), []ai.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestOpenAIProviderName(t *testing.T) {
	provider := ai.NewOpenAIProvider("http://localhost", "key", "model", 10)
	if provider.Name() != "openai-compatible" {
		t.Fatalf("expected name openai-compatible, got %s", provider.Name())
	}
}
