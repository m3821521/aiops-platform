package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/ai"
)

// mockProvider 是测试用 AI Provider，可控制返回内容和阻塞行为。
type mockProvider struct {
	responses []string // 按顺序返回的响应
	callCount int
	block     time.Duration // 每次调用阻塞时间（模拟慢 LLM）
	err       error         // 如果设置，每次调用返回此错误
}

func (m *mockProvider) Name() string { return "mock-provider" }

func (m *mockProvider) Chat(ctx context.Context, messages []ai.Message) (string, error) {
	m.callCount++
	// 如果设置了阻塞时间，模拟慢 LLM，同时尊重 context cancellation
	if m.block > 0 {
		select {
		case <-time.After(m.block):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if m.err != nil {
		return "", m.err
	}
	if m.callCount <= len(m.responses) {
		return m.responses[m.callCount-1], nil
	}
	// 默认返回最终回答
	return `{"answer": "分析完成", "summary": "测试摘要", "root_cause": "测试根因", "confidence": 0.9}`, nil
}

// blockingTool 是会阻塞的测试用 Tool，用于测试 Tool timeout。
type blockingTool struct {
	name      string
	block     time.Duration
	callCount int
}

func (t *blockingTool) Name() string        { return t.name }
func (t *blockingTool) Description() string { return "blocking mock tool" }
func (t *blockingTool) InputSchema() ToolSchema {
	return ToolSchema{Type: "object", Properties: map[string]ToolProperty{"x": {Type: "string"}}}
}
func (t *blockingTool) ReadOnly() bool { return true }
func (t *blockingTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	t.callCount++
	if t.block > 0 {
		select {
		case <-time.After(t.block):
		case <-ctx.Done():
			return ToolResult{}, ctx.Err()
		}
	}
	return ToolResult{ToolName: t.name, Success: true, Data: "ok"}, nil
}

// TestEngine_NormalSuccessfulRequest 测试正常 AI 请求成功完成。
func TestEngine_NormalSuccessfulRequest(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"answer": "最终回答", "summary": "摘要", "root_cause": "根因", "confidence": 0.85}`,
		},
	}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.Response.Answer != "最终回答" {
		t.Errorf("expected answer '最终回答', got '%s'", result.Response.Answer)
	}
	if provider.callCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", provider.callCount)
	}
}

// TestEngine_OverallTimeout 测试 overall context timeout 时 Engine 立即停止。
func TestEngine_OverallTimeout(t *testing.T) {
	// Provider 每次阻塞 500ms，overall timeout 设为 100ms
	provider := &mockProvider{block: 500 * time.Millisecond}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := engine.Ask(ctx, "测试问题", "")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrRequestTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected ErrRequestTimeout or DeadlineExceeded, got: %v", err)
	}
	// 确认 LLM 只被调用了 1 次（timeout 后不应该继续调用）
	if provider.callCount > 1 {
		t.Errorf("expected at most 1 LLM call after timeout, got %d", provider.callCount)
	}
}

// TestEngine_ClientCancellation 测试客户端主动取消时 Engine 立即停止。
func TestEngine_ClientCancellation(t *testing.T) {
	provider := &mockProvider{block: 500 * time.Millisecond}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	ctx, cancel := context.WithCancel(context.Background())
	// 100ms 后取消
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := engine.Ask(ctx, "测试问题", "")
	if err == nil {
		t.Fatal("expected canceled error, got nil")
	}
	if !errors.Is(err, ErrRequestCanceled) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected ErrRequestCanceled or Canceled, got: %v", err)
	}
	if provider.callCount > 1 {
		t.Errorf("expected at most 1 LLM call after cancel, got %d", provider.callCount)
	}
}

// TestEngine_LLMTimeoutPropagation 测试 LLM 调用超时通过 context 传播。
func TestEngine_LLMTimeoutPropagation(t *testing.T) {
	// Provider 阻塞 200ms，overall timeout 100ms
	provider := &mockProvider{block: 200 * time.Millisecond}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := engine.Ask(ctx, "测试问题", "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 确认在 timeout 附近返回（不应该等待完整 200ms）
	if elapsed > 180*time.Millisecond {
		t.Errorf("expected return near 100ms timeout, got %v", elapsed)
	}
}

// TestEngine_ToolTimeoutPropagation 测试单个 Tool 超时不阻塞整体请求。
func TestEngine_ToolTimeoutPropagation(t *testing.T) {
	// 第一轮 LLM 返回 Tool 调用，Tool 阻塞 200ms，Tool timeout 50ms
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "slow_tool", "input": {"x": "y"}}`,
			`{"answer": "完成", "summary": "摘要", "root_cause": "根因", "confidence": 0.8}`,
		},
	}
	registry := NewRegistry()
	slowTool := &blockingTool{name: "slow_tool", block: 200 * time.Millisecond}
	_ = registry.Register(slowTool)

	config := DefaultEngineConfig()
	config.ToolTimeout = 50 * time.Millisecond
	engine := NewEngine(provider, registry, config)

	ctx := context.Background()
	start := time.Now()
	result, err := engine.Ask(ctx, "测试问题", "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success (Tool timeout is non-fatal), got error: %v", err)
	}
	// Tool 应该在 50ms 左右超时，不应该等待完整 200ms
	if elapsed > 150*time.Millisecond {
		t.Errorf("expected return near 50ms tool timeout, got %v", elapsed)
	}
	if slowTool.callCount != 1 {
		t.Errorf("expected 1 tool call, got %d", slowTool.callCount)
	}
	if result.Response.Answer != "完成" {
		t.Errorf("expected answer '完成', got '%s'", result.Response.Answer)
	}
}

// TestEngine_TimeoutStopsFurtherRounds 测试 timeout 后不进入下一轮。
func TestEngine_TimeoutStopsFurtherRounds(t *testing.T) {
	// Provider 每次阻塞 80ms，overall timeout 100ms
	// 第一轮应该在 80ms 返回 Tool 调用，然后 Tool 执行时 timeout
	provider := &mockProvider{
		block: 80 * time.Millisecond,
		responses: []string{
			`{"tool_name": "fast_tool", "input": {"x": "y"}}`,
		},
	}
	registry := NewRegistry()
	fastTool := &blockingTool{name: "fast_tool", block: 10 * time.Millisecond}
	_ = registry.Register(fastTool)

	engine := NewEngine(provider, registry, DefaultEngineConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := engine.Ask(ctx, "测试问题", "")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// LLM 应该只被调用 1-2 次（第一轮 + 可能的第二轮开始前 timeout）
	// 不应该调用 8 次（MaxToolCalls）
	if provider.callCount > 3 {
		t.Errorf("expected at most 3 LLM calls before timeout, got %d (timeout should stop further rounds)", provider.callCount)
	}
}

// TestEngine_NoFakeSuccessOnError 测试错误时不返回 fake success。
func TestEngine_NoFakeSuccessOnError(t *testing.T) {
	provider := &mockProvider{err: errors.New("LLM service unavailable")}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err == nil {
		t.Fatal("expected error, got nil (fake success detected)")
	}
	if result != nil {
		t.Error("expected nil result on error, got non-nil result")
	}
}

// TestEngine_ContextCheckedBeforeEachRound 测试每轮开始前检查 context。
func TestEngine_ContextCheckedBeforeEachRound(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "fast_tool", "input": {"x": "y"}}`,
		},
	}
	registry := NewRegistry()
	fastTool := &blockingTool{name: "fast_tool", block: 1 * time.Millisecond}
	_ = registry.Register(fastTool)

	engine := NewEngine(provider, registry, DefaultEngineConfig())

	// 使用已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := engine.Ask(ctx, "测试问题", "")
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
	if !errors.Is(err, ErrRequestCanceled) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected ErrRequestCanceled or Canceled, got: %v", err)
	}
	// 确认 LLM 没有被调用（context 在第一轮开始前就被取消了）
	if provider.callCount > 0 {
		t.Errorf("expected 0 LLM calls for pre-canceled context, got %d", provider.callCount)
	}
}

// TestClassifyContextError 测试 classifyContextError 正确分类错误。
func TestClassifyContextError(t *testing.T) {
	// DeadlineExceeded + ctx deadline exceeded → ErrRequestTimeout
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel1()
	time.Sleep(5 * time.Millisecond)
	err1 := classifyContextError(ctx1, context.DeadlineExceeded)
	if !errors.Is(err1, ErrRequestTimeout) {
		t.Errorf("expected ErrRequestTimeout, got: %v", err1)
	}

	// Canceled → ErrRequestCanceled
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	err2 := classifyContextError(ctx2, context.Canceled)
	if !errors.Is(err2, ErrRequestCanceled) {
		t.Errorf("expected ErrRequestCanceled, got: %v", err2)
	}

	// 普通错误保持不变
	ctx3 := context.Background()
	originalErr := errors.New("some other error")
	err3 := classifyContextError(ctx3, originalErr)
	if !errors.Is(err3, originalErr) {
		t.Errorf("expected original error, got: %v", err3)
	}
}
