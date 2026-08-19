package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider 是 OpenAI 兼容 API 的实现。
// 支持 OpenAI、Azure OpenAI、Ollama、vLLM 及其他 OpenAI Compatible API。
// 使用标准库 net/http，不引入额外 SDK。
type OpenAIProvider struct {
	http    *http.Client
	baseURL string
	apiKey  string
	model   string
}

// NewOpenAIProvider 创建 OpenAI 兼容 Provider。
func NewOpenAIProvider(baseURL, apiKey, model string, timeoutSec int) *OpenAIProvider {
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	return &OpenAIProvider{
		http:    &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai-compatible"
}

// openAIRequest 是 OpenAI Chat Completions 请求体。
type openAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// openAIResponse 是 OpenAI Chat Completions 响应体。
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Chat 发送对话请求。
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	if p == nil || p.baseURL == "" {
		return "", fmt.Errorf("LLM 未配置")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("消息不能为空")
	}

	reqBody := openAIRequest{
		Model:    p.model,
		Messages: messages,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 LLM 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("LLM 返回 %d: %s", resp.StatusCode, string(respBytes))
	}

	var result openAIResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("LLM 错误: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空结果")
	}

	return result.Choices[0].Message.Content, nil
}
