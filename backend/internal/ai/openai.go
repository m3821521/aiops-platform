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

// SetAPIKey 运行时更新 API Key（用于前端配置）。
func (p *OpenAIProvider) SetAPIKey(apiKey string) {
	p.apiKey = apiKey
}

// SetModel 运行时更新模型名称。
func (p *OpenAIProvider) SetModel(model string) {
	if model != "" {
		p.model = model
	}
}

// SetBaseURL 运行时更新 API 地址。
func (p *OpenAIProvider) SetBaseURL(baseURL string) {
	if baseURL != "" {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
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
		// 解析错误响应，提取友好的错误信息，不泄露 API Key
		var errResp openAIResponse
		_ = json.Unmarshal(respBytes, &errResp)
		switch resp.StatusCode {
		case 401:
			return "", fmt.Errorf("API Key 无效或已过期，请在 AI 配置中检查你的 API Key")
		case 403:
			return "", fmt.Errorf("API Key 权限不足，请检查账户权限")
		case 429:
			return "", fmt.Errorf("API 请求频率超限，请稍后重试")
		case 404:
			return "", fmt.Errorf("模型不存在或 API 地址错误，请检查模型名称和 API 地址")
		default:
			if errResp.Error != nil && errResp.Error.Message != "" {
				// 只返回错误消息，不返回完整 JSON（避免泄露 API Key）
				return "", fmt.Errorf("AI 服务错误: %s", errResp.Error.Message)
			}
			return "", fmt.Errorf("AI 服务返回错误 (HTTP %d)，请稍后重试", resp.StatusCode)
		}
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
