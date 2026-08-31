package ai

import (
	"bufio"
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
	Stream   bool      `json:"stream,omitempty"`
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

// openAIStreamChunk 是 OpenAI Streaming 响应的单个 chunk。
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// StreamChunk 是 Provider streaming 的通用 chunk 结构。
type StreamChunk struct {
	Text string // 当前 chunk 的文本内容
	Done bool   // 是否为最后一个 chunk
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

// ChatStream 发送流式对话请求，通过 callback 逐 chunk 返回文本。
// 这是真正的 Provider streaming：从 DeepSeek 收到一个 chunk 就立即调用 callback，
// 而不是等待完整响应后再拆分。
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, callback func(StreamChunk) error) error {
	if p == nil || p.baseURL == "" {
		return fmt.Errorf("LLM 未配置")
	}
	if len(messages) == 0 {
		return fmt.Errorf("消息不能为空")
	}

	reqBody := openAIRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	// Streaming 使用独立的 55s HTTP client，确保 Provider timeout < Backend overall timeout (60s)
	// 避免两者同时到期导致的错误分类竞争
	streamClient := &http.Client{
		Timeout: 55 * time.Second,
		// 复用底层 Transport 以保持连接池
		Transport: p.http.Transport,
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		// 区分 context canceled 和其他错误
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("请求已取消: %w", ctx.Err())
		}
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("请求超时: %w", ctx.Err())
		}
		return fmt.Errorf("请求 LLM 失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理 HTTP 错误状态码
	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		var errResp openAIResponse
		_ = json.Unmarshal(respBytes, &errResp)
		switch resp.StatusCode {
		case 401:
			return fmt.Errorf("API Key 无效或已过期，请在 AI 配置中检查你的 API Key")
		case 403:
			return fmt.Errorf("API Key 权限不足，请检查账户权限")
		case 429:
			return fmt.Errorf("API 请求频率超限，请稍后重试")
		case 404:
			return fmt.Errorf("模型不存在或 API 地址错误，请检查模型名称和 API 地址")
		default:
			if errResp.Error != nil && errResp.Error.Message != "" {
				return fmt.Errorf("AI 服务错误: %s", errResp.Error.Message)
			}
			return fmt.Errorf("AI 服务返回错误 (HTTP %d)，请稍后重试", resp.StatusCode)
		}
	}

	// 逐行读取 SSE 响应
	reader := bufio.NewReader(resp.Body)
	for {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				return fmt.Errorf("请求已取消: %w", ctx.Err())
			}
			return fmt.Errorf("请求超时: %w", ctx.Err())
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			// 读取错误可能是因为 context 取消
			if ctx.Err() == context.Canceled {
				return fmt.Errorf("请求已取消: %w", ctx.Err())
			}
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("请求超时: %w", ctx.Err())
			}
			return fmt.Errorf("读取流式响应失败: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// SSE 格式: "data: {...}"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)

		// 流结束标记
		if data == "[DONE]" {
			break
		}

		// 解析 chunk JSON
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// 跳过无法解析的行，不中断整个流
			continue
		}

		// 检查 Provider 错误
		if chunk.Error != nil && chunk.Error.Message != "" {
			return fmt.Errorf("AI 服务错误: %s", chunk.Error.Message)
		}

		// 提取文本内容
		if len(chunk.Choices) > 0 {
			text := chunk.Choices[0].Delta.Content
			if text != "" {
				if err := callback(StreamChunk{Text: text, Done: false}); err != nil {
					return fmt.Errorf("处理流式 chunk 失败: %w", err)
				}
			}
			// 检查是否完成
			if chunk.Choices[0].FinishReason != "" {
				break
			}
		}
	}

	// 发送完成信号
	if err := callback(StreamChunk{Text: "", Done: true}); err != nil {
		return fmt.Errorf("发送流式完成信号失败: %w", err)
	}

	return nil
}
