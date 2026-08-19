package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
)

// Assistant 是 AI 运维助手。
// 它接收用户的自然语言问题，收集运维上下文，调用 LLM 进行分析。
type Assistant struct {
	provider     Provider
	alertRepo    *alert.Repository
	systemPrompt string
}

// NewAssistant 创建 AI 助手。
func NewAssistant(provider Provider, alertRepo *alert.Repository) *Assistant {
	return &Assistant{
		provider:     provider,
		alertRepo:    alertRepo,
		systemPrompt: defaultSystemPrompt(),
	}
}

// AskRequest 是 AI 助手的请求。
type AskRequest struct {
	Question string `json:"question"`
	Service  string `json:"service,omitempty"`  // 可选：指定服务
	Duration string `json:"duration,omitempty"` // 可选：时间范围，如 "10m", "1h"
}

// AskResult 是 AI 助手的回复。
type AskResult struct {
	Answer  string    `json:"answer"`
	Context string    `json:"context,omitempty"` // 收集到的上下文摘要
	Model   string    `json:"model,omitempty"`
	AskedAt time.Time `json:"asked_at"`
}

// Ask 向 AI 助手提问。
func (a *Assistant) Ask(ctx context.Context, req AskRequest) (*AskResult, error) {
	if a.provider == nil {
		return nil, fmt.Errorf("AI 服务未配置")
	}
	if strings.TrimSpace(req.Question) == "" {
		return nil, fmt.Errorf("问题不能为空")
	}

	// 1. 收集运维上下文。
	contextSummary := a.collectContext(ctx, req)

	// 2. 构建消息。
	messages := a.buildMessages(req, contextSummary)

	// 3. 调用 LLM。
	answer, err := a.provider.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("调用 AI 模型失败: %w", err)
	}

	return &AskResult{
		Answer:  answer,
		Context: contextSummary,
		Model:   a.provider.Name(),
		AskedAt: time.Now(),
	}, nil
}

// collectContext 收集运维上下文。
// 第一版：查询最近的 firing 告警摘要。
func (a *Assistant) collectContext(ctx context.Context, req AskRequest) string {
	var parts []string

	// 查询最近 1 小时的 firing 告警。
	if a.alertRepo != nil {
		alerts, _, err := a.alertRepo.List(ctx, alert.ListFilter{
			Status: alert.StatusFiring,
		}, 1, 50)
		if err == nil && len(alerts) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("当前有 %d 条活跃告警：\n", len(alerts)))
			for i, a := range alerts {
				if i >= 20 {
					sb.WriteString(fmt.Sprintf("... 还有 %d 条告警\n", len(alerts)-20))
					break
				}
				sb.WriteString(fmt.Sprintf("- [%s] %s (service=%s, namespace=%s, severity=%s)\n",
					a.StartsAt.Format("15:04:05"), a.Alertname, a.Service, a.Namespace, a.Severity))
			}
			parts = append(parts, sb.String())
		}
	}

	// 如果指定了服务，添加提示。
	if req.Service != "" {
		parts = append(parts, fmt.Sprintf("用户关注的服务: %s", req.Service))
	}

	if len(parts) == 0 {
		return "（无额外上下文）"
	}
	return strings.Join(parts, "\n")
}

// buildMessages 构建发送给 LLM 的消息列表。
func (a *Assistant) buildMessages(req AskRequest, context string) []Message {
	return []Message{
		{
			Role:    "system",
			Content: a.systemPrompt,
		},
		{
			Role: "user",
			Content: fmt.Sprintf(`运维上下文：
%s

用户问题：%s

请根据以上上下文和你的运维知识进行分析。`, context, req.Question),
		},
	}
}

// defaultSystemPrompt 返回默认的系统提示词。
func defaultSystemPrompt() string {
	return `你是一个专业的 AIOps 智能运维助手，擅长 Kubernetes、Prometheus、ELK、数据库和微服务运维。

你的职责：
1. 根据用户提供的运维上下文（告警、指标、日志）分析问题
2. 给出可能的根因分析
3. 列出支持结论的证据
4. 提供可执行的排查和修复建议

回答格式要求：
## 问题分析
简要描述问题现象。

## 可能原因
列出 1-3 个最可能的原因，按可能性排序。

## 证据
列出支持结论的具体证据（告警、指标异常、日志等）。

## 建议
给出可执行的排查和修复步骤。

注意事项：
- 如果上下文信息不足，明确说明需要补充哪些信息
- 不要编造不存在的告警或指标
- 建议要具体、可操作，不要泛泛而谈
- 涉及危险操作（删除、重启、扩容）时，提醒用户确认后再执行`
}
