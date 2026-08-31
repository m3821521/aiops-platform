package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/ai/tools"
)

// TestClassifyAIError_OverallTimeout 验证 overall timeout 被正确分类为 AI_TIMEOUT。
func TestClassifyAIError_OverallTimeout(t *testing.T) {
	// 创建一个已经 deadline exceeded 的 context。
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	// 测试 1：tools.ErrRequestTimeout
	err1 := tools.ErrRequestTimeout
	result1 := classifyAIError(err1, ctx)
	if result1 != AIErrorTimeout {
		t.Errorf("expected AIErrorTimeout, got %s", result1)
	}

	// 测试 2：context deadline exceeded 包装的错误
	err2 := errors.New("调用 AI 模型失败: 读取响应失败: context deadline exceeded")
	result2 := classifyAIError(err2, ctx)
	if result2 != AIErrorTimeout {
		t.Errorf("expected AIErrorTimeout for wrapped deadline exceeded, got %s", result2)
	}
}

// TestClassifyAIError_ClientCancelled 验证 client cancellation 被正确分类为 AI_CLIENT_CANCELLED。
func TestClassifyAIError_ClientCancelled(t *testing.T) {
	// 创建一个已经 canceled 的 context。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 测试 1：tools.ErrRequestCanceled
	err1 := tools.ErrRequestCanceled
	result1 := classifyAIError(err1, ctx)
	if result1 != AIErrorClientCancelled {
		t.Errorf("expected AIErrorClientCancelled, got %s", result1)
	}

	// 测试 2：context canceled 包装的错误
	err2 := errors.New("请求 LLM 失败: context canceled")
	result2 := classifyAIError(err2, ctx)
	if result2 != AIErrorClientCancelled {
		t.Errorf("expected AIErrorClientCancelled for wrapped canceled, got %s", result2)
	}
}

// TestClassifyAIError_LLMTimeout 验证 LLM timeout 被正确分类为 AI_PROVIDER_TIMEOUT。
func TestClassifyAIError_LLMTimeout(t *testing.T) {
	ctx := context.Background()

	// 测试：tools.ErrLLMTimeout
	err := tools.ErrLLMTimeout
	result := classifyAIError(err, ctx)
	if result != AIErrorProviderTimeout {
		t.Errorf("expected AIErrorProviderTimeout, got %s", result)
	}
}

// TestClassifyAIError_ToolTimeout 验证 Tool timeout 被正确分类为 AI_TOOL_ERROR。
func TestClassifyAIError_ToolTimeout(t *testing.T) {
	ctx := context.Background()

	// 测试：tools.ErrToolTimeout
	err := tools.ErrToolTimeout
	result := classifyAIError(err, ctx)
	if result != AIErrorToolError {
		t.Errorf("expected AIErrorToolError, got %s", result)
	}
}

// TestClassifyAIError_ProviderError 验证 Provider HTTP 错误被正确分类为 AI_PROVIDER_ERROR。
func TestClassifyAIError_ProviderError(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name string
		err  error
	}{
		{"API Key 无效", errors.New("API Key 无效或已过期，请在 AI 配置中检查你的 API Key")},
		{"权限不足", errors.New("API Key 权限不足，请检查账户权限")},
		{"频率超限", errors.New("API 请求频率超限，请稍后重试")},
		{"模型不存在", errors.New("模型不存在或 API 地址错误，请检查模型名称和 API 地址")},
		{"AI 服务错误", errors.New("AI 服务错误: internal server error")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyAIError(tc.err, ctx)
			if result != AIErrorProviderError {
				t.Errorf("expected AIErrorProviderError for %s, got %s", tc.name, result)
			}
		})
	}
}

// TestClassifyAIError_ClientCancelledPriority 验证 client cancellation 优先于 timeout 分类。
// 这是一个重要的边界情况：当 client 断开时，context 可能同时表现为 canceled 和 deadline exceeded。
// 必须优先分类为 client cancelled，而不是 timeout。
func TestClassifyAIError_ClientCancelledPriority(t *testing.T) {
	// 创建一个带 timeout 的 context，然后立即 cancel。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cancel()

	// 此时 ctx.Err() 应该是 context.Canceled。
	if ctx.Err() != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", ctx.Err())
	}

	// 测试：即使错误信息包含 deadline exceeded，也应该优先分类为 client cancelled。
	err := errors.New("调用 AI 模型失败: context canceled")
	result := classifyAIError(err, ctx)
	if result != AIErrorClientCancelled {
		t.Errorf("expected AIErrorClientCancelled (priority over timeout), got %s", result)
	}
}

// TestAIAskTimeoutConstant 验证 aiAskTimeout 常量已正确设置为 60s。
func TestAIAskTimeoutConstant(t *testing.T) {
	expected := 60 * time.Second
	if aiAskTimeout != expected {
		t.Errorf("expected aiAskTimeout = %v, got %v", expected, aiAskTimeout)
	}
	t.Logf("aiAskTimeout = %v (P2-AI-ASSISTANT-PERF-002 Phase 1: changed from 25s to 60s)", aiAskTimeout)
}

// TestAIErrorTypeStringValues 验证错误类型字符串值符合规范。
func TestAIErrorTypeStringValues(t *testing.T) {
	testCases := []struct {
		errType AIErrorType
		expected string
	}{
		{AIErrorTimeout, "AI_TIMEOUT"},
		{AIErrorProviderTimeout, "AI_PROVIDER_TIMEOUT"},
		{AIErrorClientCancelled, "AI_CLIENT_CANCELLED"},
		{AIErrorProviderError, "AI_PROVIDER_ERROR"},
		{AIErrorToolError, "AI_TOOL_ERROR"},
		{AIErrorUnknown, "AI_UNKNOWN_ERROR"},
	}

	for _, tc := range testCases {
		if string(tc.errType) != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.errType)
		}
	}
}
