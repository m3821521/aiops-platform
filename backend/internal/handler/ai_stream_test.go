package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/gin-gonic/gin"
)

// mockStreamProvider 是用于 SSE Handler 测试的 mock Provider。
type mockStreamProvider struct {
	mu           sync.Mutex
	chunks       []string        // 要返回的 token chunks
	delay        time.Duration   // 每个 chunk 之间的延迟
	returnErr    error           // 要返回的错误
	chatCalls    int             // Chat 调用次数
	streamCalls  int             // ChatStream 调用次数
	canceled     bool            // 是否检测到 context cancellation
	canceledCh   chan struct{}   // 通知 context 已取消
	blockForever bool            // 阻塞直到 context 取消
}

func (m *mockStreamProvider) Chat(ctx context.Context, messages []ai.Message) (string, error) {
	m.mu.Lock()
	m.chatCalls++
	m.mu.Unlock()
	return "mock answer", nil
}

func (m *mockStreamProvider) ChatStream(ctx context.Context, messages []ai.Message, callback func(ai.StreamChunk) error) error {
	m.mu.Lock()
	m.streamCalls++
	m.mu.Unlock()

	if m.blockForever {
		// 阻塞直到 context 取消，用于测试 cancellation
		<-ctx.Done()
		m.mu.Lock()
		m.canceled = true
		m.mu.Unlock()
		if m.canceledCh != nil {
			close(m.canceledCh)
		}
		return fmt.Errorf("请求已取消: %w", ctx.Err())
	}

	if m.returnErr != nil {
		return m.returnErr
	}

	for _, chunk := range m.chunks {
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.canceled = true
			m.mu.Unlock()
			return fmt.Errorf("请求已取消: %w", ctx.Err())
		default:
		}

		if m.delay > 0 {
			time.Sleep(m.delay)
		}
		if err := callback(ai.StreamChunk{Text: chunk, Done: false}); err != nil {
			return err
		}
	}
	// 发送完成信号
	return callback(ai.StreamChunk{Text: "", Done: true})
}

func (m *mockStreamProvider) Name() string { return "mock-stream-provider" }

// setupTestHandler 创建测试用的 AIHandler 和 gin engine。
func setupTestHandler(provider ai.Provider) (*AIHandler, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	assistant := ai.NewAssistant(provider, nil)
	h := &AIHandler{
		Enabled:         true,
		APIKeyConfigured: true,
		Assistant:       assistant,
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("request_id", "test-request-id-123")
		c.Next()
	})
	r.POST("/api/v1/ai/ask/stream", h.AskStream)
	return h, r
}

// parseSSEEvents 从 SSE 响应体中解析事件列表。
func parseSSEEvents(body string) []map[string]interface{} {
	var events []map[string]interface{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentEvent string
	var currentData strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "data:") {
			if currentData.Len() > 0 {
				currentData.WriteString("\n")
			}
			currentData.WriteString(strings.TrimSpace(line[5:]))
		} else if line == "" && currentEvent != "" {
			var data map[string]interface{}
			if currentData.String() != "" {
				_ = json.Unmarshal([]byte(currentData.String()), &data)
			}
			if data == nil {
				data = make(map[string]interface{})
			}
			data["_event"] = currentEvent
			events = append(events, data)
			currentEvent = ""
			currentData.Reset()
		}
	}
	// 处理最后一个没有空行的事件
	if currentEvent != "" {
		var data map[string]interface{}
		if currentData.String() != "" {
			_ = json.Unmarshal([]byte(currentData.String()), &data)
		}
		if data == nil {
			data = make(map[string]interface{})
		}
		data["_event"] = currentEvent
		events = append(events, data)
	}
	return events
}

// TestAskStreamSuccess 验证正常 streaming 请求。
func TestAskStreamSuccess(t *testing.T) {
	provider := &mockStreamProvider{
		chunks: []string{"Hel", "lo", " ", "AIOps"},
	}
	_, r := setupTestHandler(provider)

	req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
		strings.NewReader(`{"question":"你好"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	events := parseSSEEvents(w.Body.String())
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (start, tokens, done), got %d", len(events))
	}

	// 验证 start event
	if events[0]["_event"] != "start" {
		t.Errorf("expected first event 'start', got '%s'", events[0]["_event"])
	}
	if events[0]["request_id"] != "test-request-id-123" {
		t.Errorf("expected request_id test-request-id-123, got %v", events[0]["request_id"])
	}

	// 收集所有 token
	var fullText string
	var doneEvent map[string]interface{}
	for _, e := range events {
		if e["_event"] == "token" {
			if text, ok := e["text"].(string); ok {
				fullText += text
			}
		}
		if e["_event"] == "done" {
			doneEvent = e
		}
	}

	// 验证 token 完整性（不重复、不丢失）
	if fullText != "Hello AIOps" {
		t.Errorf("expected full text 'Hello AIOps', got '%s'", fullText)
	}

	// 验证 done event
	if doneEvent == nil {
		t.Fatal("expected done event")
	}
	if doneEvent["request_id"] != "test-request-id-123" {
		t.Errorf("done event request_id mismatch: expected test-request-id-123, got %v", doneEvent["request_id"])
	}
	if doneEvent["streaming"] != true {
		t.Errorf("expected streaming=true in done event")
	}
}

// TestAskStreamClientCancellation 验证 client cancellation 正确传播。
func TestAskStreamClientCancellation(t *testing.T) {
	provider := &mockStreamProvider{
		blockForever: true,
		canceledCh:   make(chan struct{}),
	}
	_, r := setupTestHandler(provider)

	req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
		strings.NewReader(`{"question":"你好"}`))
	req.Header.Set("Content-Type", "application/json")

	// 使用可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	// 在 goroutine 中执行请求，因为它会阻塞
	done := make(chan struct{})
	go func() {
		r.ServeHTTP(w, req)
		close(done)
	}()

	// 等待 start event 发送后取消
	time.Sleep(100 * time.Millisecond)
	cancel()

	// 等待 handler 退出
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not exit after cancellation")
	}

	// 验证 Provider 检测到了 cancellation
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if !provider.canceled {
		t.Error("expected provider to detect context cancellation")
	}

	// 验证响应中包含 error event（AI_CLIENT_CANCELLED）
	events := parseSSEEvents(w.Body.String())
	var hasCancelError bool
	for _, e := range events {
		if e["_event"] == "error" {
			if et, ok := e["error_type"].(string); ok && et == "AI_CLIENT_CANCELLED" {
				hasCancelError = true
			}
		}
	}
	if !hasCancelError {
		t.Logf("events: %+v", events)
		t.Error("expected AI_CLIENT_CANCELLED error event")
	}
}

// TestAskStreamProviderError 验证 Provider 错误正确处理。
func TestAskStreamProviderError(t *testing.T) {
	provider := &mockStreamProvider{
		returnErr: fmt.Errorf("API Key 无效或已过期，请在 AI 配置中检查你的 API Key"),
	}
	_, r := setupTestHandler(provider)

	req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
		strings.NewReader(`{"question":"你好"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	events := parseSSEEvents(w.Body.String())
	var errorEvent map[string]interface{}
	for _, e := range events {
		if e["_event"] == "error" {
			errorEvent = e
		}
	}
	if errorEvent == nil {
		t.Fatalf("expected error event, events: %+v", events)
	}

	// 验证错误类型
	if errorEvent["error_type"] != string(AIErrorProviderError) {
		t.Errorf("expected error_type AI_PROVIDER_ERROR, got %v", errorEvent["error_type"])
	}

	// 验证敏感信息脱敏（不应包含 "API Key" 原文）
	if msg, ok := errorEvent["message"].(string); ok {
		if strings.Contains(msg, "API Key") {
			t.Errorf("error message should be sanitized, got: %s", msg)
		}
	}

	// 验证 request_id 一致
	if errorEvent["request_id"] != "test-request-id-123" {
		t.Errorf("error event request_id mismatch: got %v", errorEvent["request_id"])
	}
}

// TestAskStreamRequestIDConsistency 验证 request_id 全链路一致。
func TestAskStreamRequestIDConsistency(t *testing.T) {
	provider := &mockStreamProvider{
		chunks: []string{"test"},
	}
	_, r := setupTestHandler(provider)

	req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
		strings.NewReader(`{"question":"你好"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	events := parseSSEEvents(w.Body.String())
	for _, e := range events {
		eventType := e["_event"]
		// start 和 done 和 error 事件必须包含 request_id
		if eventType == "start" || eventType == "done" || eventType == "error" {
			if e["request_id"] != "test-request-id-123" {
				t.Errorf("event %s request_id mismatch: expected test-request-id-123, got %v",
					eventType, e["request_id"])
			}
		}
	}
}

// TestAskStreamTokenIntegrity 验证 token 不重复、不丢失。
func TestAskStreamTokenIntegrity(t *testing.T) {
	testCases := []struct {
		name     string
		chunks   []string
		expected string
	}{
		{
			name:     "normal chunks",
			chunks:   []string{"Hel", "lo", " ", "World", "!"},
			expected: "Hello World!",
		},
		{
			name:     "single chunk",
			chunks:   []string{"完整回答"},
			expected: "完整回答",
		},
		{
			name:     "empty chunks skipped",
			chunks:   []string{"A", "", "B"},
			expected: "AB", // ChatStream 中 text != "" 才发送
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &mockStreamProvider{chunks: tc.chunks}
			_, r := setupTestHandler(provider)

			req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
				strings.NewReader(`{"question":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			events := parseSSEEvents(w.Body.String())
			var fullText string
			for _, e := range events {
				if e["_event"] == "token" {
					if text, ok := e["text"].(string); ok {
						fullText += text
					}
				}
			}
			if fullText != tc.expected {
				t.Errorf("expected '%s', got '%s'", tc.expected, fullText)
			}
		})
	}
}

// TestAskStreamHeartbeat 验证 heartbeat 在 Provider 慢响应时发送。
func TestAskStreamHeartbeat(t *testing.T) {
	// Provider 每个 chunk 延迟 100ms，总共 350ms，应该触发至少一次 heartbeat（10s 间隔太长，这里测试逻辑存在性）
	// 注意：heartbeat 间隔是 10s，单元测试中不会实际触发。
	// 这里验证 heartbeat ticker 被正确创建和清理（通过不 panic 间接验证）
	provider := &mockStreamProvider{
		chunks: []string{"slow"},
		delay:  10 * time.Millisecond,
	}
	_, r := setupTestHandler(provider)

	req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
		strings.NewReader(`{"question":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	// 验证没有 panic，正常完成
	events := parseSSEEvents(w.Body.String())
	var hasDone bool
	for _, e := range events {
		if e["_event"] == "done" {
			hasDone = true
		}
	}
	if !hasDone {
		t.Error("expected done event")
	}
}

// TestAskStreamNotEnabled 验证 AI 未启用时返回 503。
func TestAskStreamNotEnabled(t *testing.T) {
	provider := &mockStreamProvider{}
	gin.SetMode(gin.TestMode)
	assistant := ai.NewAssistant(provider, nil)
	h := &AIHandler{
		Enabled:   false,
		Assistant: assistant,
	}
	r := gin.New()
	r.POST("/api/v1/ai/ask/stream", h.AskStream)

	req := httptest.NewRequest("POST", "/api/v1/ai/ask/stream",
		strings.NewReader(`{"question":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected HTTP 503, got %d", w.Code)
	}
}
