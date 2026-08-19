package ai_test

import (
	"context"
	"testing"

	"github.com/aiops/aiops-platform/internal/ai"
)

// mockProvider 是一个测试用的 LLM Provider。
type mockProvider struct {
	reply    string
	err      error
	received []ai.Message
}

func (m *mockProvider) Chat(_ context.Context, messages []ai.Message) (string, error) {
	m.received = messages
	if m.err != nil {
		return "", m.err
	}
	return m.reply, nil
}

func (m *mockProvider) Name() string {
	return "mock"
}

func TestAssistantAsk(t *testing.T) {
	provider := &mockProvider{reply: "分析结果：服务正常"}
	assistant := ai.NewAssistant(provider, nil)

	result, err := assistant.Ask(context.Background(), ai.AskRequest{
		Question: "order-service 为什么错误率升高？",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "分析结果：服务正常" {
		t.Fatalf("unexpected answer: %s", result.Answer)
	}
	if result.Model != "mock" {
		t.Fatalf("expected model mock, got %s", result.Model)
	}
	// 验证发送了 system 和 user 两条消息。
	if len(provider.received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(provider.received))
	}
	if provider.received[0].Role != "system" {
		t.Fatalf("expected first message role system, got %s", provider.received[0].Role)
	}
	if provider.received[1].Role != "user" {
		t.Fatalf("expected second message role user, got %s", provider.received[1].Role)
	}
}

func TestAssistantAskEmptyQuestion(t *testing.T) {
	provider := &mockProvider{}
	assistant := ai.NewAssistant(provider, nil)

	_, err := assistant.Ask(context.Background(), ai.AskRequest{Question: "   "})
	if err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestAssistantAskNoProvider(t *testing.T) {
	assistant := ai.NewAssistant(nil, nil)
	_, err := assistant.Ask(context.Background(), ai.AskRequest{Question: "test"})
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestAssistantAskProviderError(t *testing.T) {
	provider := &mockProvider{err: context.DeadlineExceeded}
	assistant := ai.NewAssistant(provider, nil)

	_, err := assistant.Ask(context.Background(), ai.AskRequest{Question: "test"})
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestAssistantAskWithService(t *testing.T) {
	provider := &mockProvider{reply: "ok"}
	assistant := ai.NewAssistant(provider, nil)

	result, err := assistant.Ask(context.Background(), ai.AskRequest{
		Question: "为什么慢？",
		Service:  "order-service",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 上下文中应该包含服务名。
	if result.Context == "" {
		t.Fatal("expected context to include service info")
	}
}
