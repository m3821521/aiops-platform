package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/ai"
)

// ===== Phase 2 Optimization Tests =====

// TestEngine_EarlyStopAfterFinalAnswer 测试 LLM 返回最终回答时立即结束，不进入下一轮。
func TestEngine_EarlyStopAfterFinalAnswer(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"answer": "最终回答", "summary": "摘要", "root_cause": "根因", "confidence": 0.9}`,
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
		t.Errorf("expected exactly 1 LLM call (early stop), got %d", provider.callCount)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(result.ToolCalls))
	}
}

// TestEngine_NoToolCallStopsImmediately 测试 LLM 不请求 Tool 时立即返回最终回答。
func TestEngine_NoToolCallStopsImmediately(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"answer": "不需要工具，直接回答", "summary": "摘要", "confidence": 0.8}`,
		},
	}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "简单问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if provider.callCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", provider.callCount)
	}
	if result.Response.Answer != "不需要工具，直接回答" {
		t.Errorf("unexpected answer: %s", result.Response.Answer)
	}
}

// TestEngine_DuplicateToolCallSkipped 测试重复 Tool Call（相同 Tool+Input）被跳过，不重复执行。
func TestEngine_DuplicateToolCallSkipped(t *testing.T) {
	// 第一轮：LLM 请求 tool_a
	// 第二轮：LLM 再次请求 tool_a（相同参数）→ 应该被跳过
	// 第三轮：LLM 返回最终回答
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "tool_a", "input": {"x": "1"}}`,
			`{"tool_name": "tool_a", "input": {"x": "1"}}`, // 重复调用
			`{"answer": "最终回答", "summary": "摘要", "confidence": 0.7}`,
		},
	}
	registry := NewRegistry()
	toolA := &countingTool{name: "tool_a"}
	_ = registry.Register(toolA)
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	// tool_a 应该只执行 1 次（第二次被跳过）
	if toolA.callCount != 1 {
		t.Errorf("expected tool_a executed 1 time (duplicate skipped), got %d", toolA.callCount)
	}
	if result.SkippedCalls != 1 {
		t.Errorf("expected 1 skipped call, got %d", result.SkippedCalls)
	}
	if provider.callCount != 3 {
		t.Errorf("expected 3 LLM calls, got %d", provider.callCount)
	}
}

// TestEngine_DuplicateCallDoesNotExecute 验证重复 Tool Call 不会真正执行 Tool。
func TestEngine_DuplicateCallDoesNotExecute(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "slow_tool", "input": {"x": "1"}}`,
			`{"tool_name": "slow_tool", "input": {"x": "1"}}`, // 重复
			`{"answer": "完成", "summary": "摘要", "confidence": 0.6}`,
		},
	}
	registry := NewRegistry()
	// slow_tool 每次执行阻塞 100ms，如果重复执行会显著增加耗时
	slowTool := &blockingTool{name: "slow_tool", block: 100 * time.Millisecond}
	_ = registry.Register(slowTool)
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	start := time.Now()
	result, err := engine.Ask(context.Background(), "测试问题", "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	// 如果重复调用被跳过，总耗时应该接近 100ms（只执行一次），而不是 200ms+
	if elapsed > 180*time.Millisecond {
		t.Errorf("expected elapsed near 100ms (duplicate skipped), got %v", elapsed)
	}
	if slowTool.callCount != 1 {
		t.Errorf("expected slow_tool executed 1 time, got %d", slowTool.callCount)
	}
	if result.SkippedCalls != 1 {
		t.Errorf("expected 1 skipped call, got %d", result.SkippedCalls)
	}
}

// TestEngine_DifferentInputNotDuplicate 验证不同 Input 的相同 Tool 不算重复，会正常执行。
func TestEngine_DifferentInputNotDuplicate(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "tool_a", "input": {"x": "1"}}`,
			`{"tool_name": "tool_a", "input": {"x": "2"}}`, // 不同参数，不算重复
			`{"answer": "完成", "summary": "摘要", "confidence": 0.6}`,
		},
	}
	registry := NewRegistry()
	toolA := &countingTool{name: "tool_a"}
	_ = registry.Register(toolA)
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	// 不同参数应该执行 2 次
	if toolA.callCount != 2 {
		t.Errorf("expected tool_a executed 2 times (different input), got %d", toolA.callCount)
	}
	if result.SkippedCalls != 0 {
		t.Errorf("expected 0 skipped calls, got %d", result.SkippedCalls)
	}
}

// TestEngine_ToolResultSummaryTruncatesLargeData 验证大 Tool 结果被摘要截断，减少 context 膨胀。
func TestEngine_ToolResultSummaryTruncatesLargeData(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "big_tool", "input": {}}`,
			`{"answer": "完成", "summary": "摘要", "confidence": 0.6}`,
		},
	}
	registry := NewRegistry()
	// big_tool 返回 10KB 数据
	bigTool := &bigDataTool{name: "big_tool", dataSize: 10240}
	_ = registry.Register(bigTool)
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	// ToolCall.Result 应该保留完整数据（用于审计）
	if result.ToolCalls[0].Result.Data == nil {
		t.Error("expected full data retained in ToolCall record")
	}
}

// TestEngine_DependentToolsRemainSequential 验证有依赖关系的 Tool 仍然串行执行（LLM 决定顺序）。
func TestEngine_DependentToolsRemainSequential(t *testing.T) {
	// LLM 先请求 tool_a，再请求 tool_b（模拟依赖：b 依赖 a 的结果）
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "tool_a", "input": {}}`,
			`{"tool_name": "tool_b", "input": {}}`,
			`{"answer": "完成", "summary": "摘要", "confidence": 0.6}`,
		},
	}
	registry := NewRegistry()
	toolA := &countingTool{name: "tool_a"}
	toolB := &countingTool{name: "tool_b"}
	_ = registry.Register(toolA)
	_ = registry.Register(toolB)
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if toolA.callCount != 1 || toolB.callCount != 1 {
		t.Errorf("expected both tools executed once, got a=%d b=%d", toolA.callCount, toolB.callCount)
	}
	if len(result.ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(result.ToolCalls))
	}
}

// TestEngine_NoDuplicateToolCalls 验证正常流程不会产生重复 Tool 调用。
func TestEngine_NoDuplicateToolCalls(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			`{"tool_name": "tool_a", "input": {"x": "1"}}`,
			`{"tool_name": "tool_b", "input": {"y": "2"}}`,
			`{"answer": "完成", "summary": "摘要", "confidence": 0.6}`,
		},
	}
	registry := NewRegistry()
	toolA := &countingTool{name: "tool_a"}
	toolB := &countingTool{name: "tool_b"}
	_ = registry.Register(toolA)
	_ = registry.Register(toolB)
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if toolA.callCount != 1 || toolB.callCount != 1 {
		t.Errorf("expected no duplicates, got a=%d b=%d", toolA.callCount, toolB.callCount)
	}
	if result.SkippedCalls != 0 {
		t.Errorf("expected 0 skipped, got %d", result.SkippedCalls)
	}
}

// TestEngine_OverallTimeoutStillStops 验证 overall timeout 仍然能停止 Engine（Phase 2 优化不破坏 Phase 1）。
func TestEngine_OverallTimeoutStillStops(t *testing.T) {
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
		t.Errorf("expected timeout error, got: %v", err)
	}
	if provider.callCount > 1 {
		t.Errorf("expected at most 1 LLM call after timeout, got %d", provider.callCount)
	}
}

// TestEngine_ClientCancellationStopsOptimized 验证客户端取消在优化后仍然能立即停止。
func TestEngine_CancellationStopsOptimized(t *testing.T) {
	provider := &mockProvider{block: 500 * time.Millisecond}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := engine.Ask(ctx, "测试问题", "")
	if err == nil {
		t.Fatal("expected canceled error, got nil")
	}
	if !errors.Is(err, ErrRequestCanceled) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected canceled error, got: %v", err)
	}
}

// TestEngine_NoFakeSuccessOnOptimizationError 验证优化逻辑出错时不会返回 fake success。
func TestEngine_NoFakeSuccessOnOptimizationError(t *testing.T) {
	provider := &mockProvider{err: errors.New("LLM service down")}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	result, err := engine.Ask(context.Background(), "测试问题", "")
	if err == nil {
		t.Fatal("expected error, got nil (fake success detected)")
	}
	if result != nil {
		t.Error("expected nil result on error, got non-nil")
	}
}

// TestEngine_SystemPromptEncouragesDirectAnswer 验证 System Prompt 包含鼓励直接回答的提示。
func TestEngine_SystemPromptEncouragesDirectAnswer(t *testing.T) {
	provider := &captureProvider{}
	registry := NewRegistry()
	engine := NewEngine(provider, registry, DefaultEngineConfig())

	_, _ = engine.Ask(context.Background(), "测试问题", "")

	if provider.lastMessages == nil || len(provider.lastMessages) < 1 {
		t.Fatal("expected messages captured")
	}
	systemPrompt := provider.lastMessages[0].Content
	if !strings.Contains(systemPrompt, "已有足够证据") {
		t.Error("expected system prompt to contain '已有足够证据' encouragement")
	}
	if !strings.Contains(systemPrompt, "不要继续调用工具") {
		t.Error("expected system prompt to contain '不要继续调用工具'")
	}
}

// ===== Helper Test Types =====

// countingTool 是记录调用次数的测试用 Tool。
type countingTool struct {
	name      string
	callCount int
}

func (t *countingTool) Name() string        { return t.name }
func (t *countingTool) Description() string { return "counting tool" }
func (t *countingTool) InputSchema() ToolSchema {
	return ToolSchema{Type: "object", Properties: map[string]ToolProperty{"x": {Type: "string"}}}
}
func (t *countingTool) ReadOnly() bool { return true }
func (t *countingTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	t.callCount++
	return ToolResult{ToolName: t.name, Success: true, Data: "ok", Source: "test"}, nil
}

// bigDataTool 返回指定大小数据的测试用 Tool。
type bigDataTool struct {
	name     string
	dataSize int
}

func (t *bigDataTool) Name() string        { return t.name }
func (t *bigDataTool) Description() string { return "big data tool" }
func (t *bigDataTool) InputSchema() ToolSchema {
	return ToolSchema{Type: "object"}
}
func (t *bigDataTool) ReadOnly() bool { return true }
func (t *bigDataTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	bigData := strings.Repeat("x", t.dataSize)
	return ToolResult{
		ToolName:  t.name,
		Success:   true,
		Available: true,
		Data:      map[string]interface{}{"content": bigData, "size": t.dataSize},
		Source:    "test",
	}, nil
}

// captureProvider 捕获最后一次调用的 messages 的测试用 Provider。
type captureProvider struct {
	lastMessages []ai.Message
	callCount    int
}

func (p *captureProvider) Name() string { return "capture-provider" }
func (p *captureProvider) Chat(ctx context.Context, messages []ai.Message) (string, error) {
	p.lastMessages = messages
	p.callCount++
	// 返回最终回答，避免循环
	return `{"answer": "测试回答", "summary": "摘要", "confidence": 0.5}`, nil
}
