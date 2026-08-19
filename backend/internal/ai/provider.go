package ai

import "context"

// Message 是 LLM 对话中的一条消息。
type Message struct {
	Role    string `json:"role"` // system / user / assistant
	Content string `json:"content"`
}

// Provider 是 LLM 提供者接口。
// 所有 AI 模型（OpenAI、Azure、Gemini、Claude、Ollama、vLLM 等）都实现此接口。
// 以后接入新模型只需实现此接口，不需要修改 AI Assistant 核心逻辑。
type Provider interface {
	// Chat 发送对话请求，返回模型回复。
	Chat(ctx context.Context, messages []Message) (string, error)
	// Name 返回提供者名称。
	Name() string
}
