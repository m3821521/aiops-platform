package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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

// GetProvider 返回 AI 助手使用的 Provider。
func (a *Assistant) GetProvider() Provider {
	return a.provider
}

// AskRequest 是 AI 助手的请求。
type AskRequest struct {
	Question string `json:"question"`
	Service  string `json:"service,omitempty"`  // 可选：指定服务
	Duration string `json:"duration,omitempty"` // 可选：时间范围，如 "10m", "1h"
}

// AskResult 是 AI 助手的回复。
type AskResult struct {
	Answer          string           `json:"answer"`
	Context         string           `json:"context,omitempty"` // 收集到的上下文摘要
	Model           string           `json:"model,omitempty"`
	AskedAt         time.Time        `json:"asked_at"`
	Summary         string           `json:"summary,omitempty"`
	RootCause       string           `json:"root_cause,omitempty"`
	Confidence      float64          `json:"confidence,omitempty"`
	Severity        string           `json:"severity,omitempty"`
	Evidence        []EvidenceItem   `json:"evidence,omitempty"`
	PossibleCauses  []PossibleCause  `json:"possible_causes,omitempty"`
	Recommendations []Recommendation `json:"recommendations,omitempty"`
	Impact          []ImpactItem     `json:"impact,omitempty"`
}

// EvidenceItem 是证据项。
type EvidenceItem struct {
	Source      string `json:"source"`
	Description string `json:"description"`
	Resource    string `json:"resource,omitempty"`
	Importance  string `json:"importance,omitempty"`
}

// PossibleCause 是可能原因。
type PossibleCause struct {
	Cause      string `json:"cause"`
	Likelihood string `json:"likelihood,omitempty"`
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

	result := &AskResult{
		Answer:  answer,
		Context: contextSummary,
		Model:   a.provider.Name(),
		AskedAt: time.Now(),
	}

	// 4. 解析 AI 回答中的结构化 JSON。
	parseStructuredData(answer, result)

	return result, nil
}

// jsonBlockRegex 匹配 Markdown 中的 JSON 代码块。
var jsonBlockRegex = regexp.MustCompile("```(?:json)?\\s*\\n([\\s\\S]*?)\\n```")

// parseStructuredData 从 AI 回答中提取结构化 JSON 数据。
func parseStructuredData(answer string, result *AskResult) {
	// 尝试提取 JSON 代码块。
	matches := jsonBlockRegex.FindAllStringSubmatch(answer, -1)
	if len(matches) == 0 {
		return
	}

	// 取最后一个 JSON 代码块（通常是结构化数据）。
	lastMatch := matches[len(matches)-1]
	if len(lastMatch) < 2 {
		return
	}

	jsonStr := strings.TrimSpace(lastMatch[1])
	if jsonStr == "" {
		return
	}

	var structured struct {
		Summary        string           `json:"summary"`
		RootCause      string           `json:"root_cause"`
		Confidence     float64          `json:"confidence"`
		Severity       string           `json:"severity"`
		Evidence       []EvidenceItem   `json:"evidence"`
		PossibleCauses []PossibleCause  `json:"possible_causes"`
		Recommendations []Recommendation `json:"recommendations"`
		Impact         []ImpactItem     `json:"impact"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &structured); err != nil {
		// JSON 解析失败，静默忽略，仍然返回纯文本回答。
		return
	}

	result.Summary = structured.Summary
	result.RootCause = structured.RootCause
	result.Confidence = structured.Confidence
	result.Severity = structured.Severity
	result.Evidence = structured.Evidence
	result.PossibleCauses = structured.PossibleCauses
	result.Recommendations = structured.Recommendations
	result.Impact = structured.Impact
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

## 结构化数据
在回答的最后，必须附加一个 JSON 代码块，包含以下结构化字段：
` + "```json" + `
{
  "summary": "问题摘要",
  "root_cause": "最可能的根因",
  "confidence": 0.85,
  "severity": "warning|critical|info",
  "evidence": [
    {"source": "prometheus|kubernetes|alertmanager|elasticsearch", "description": "证据描述", "resource": "资源名", "importance": "high|medium|low"}
  ],
  "possible_causes": [
    {"cause": "原因描述", "likelihood": "high|medium|low"}
  ],
  "recommendations": [
    {"priority": "P0|P1|P2|P3", "title": "建议标题", "description": "详细描述", "reason": "原因", "risk": "low|medium|high|critical", "action_type": "restart_pod|scale_deployment|jenkins_build|argocd_sync", "target": "目标资源名", "namespace": "命名空间", "parameters": {"key": "value"}}
  ],
  "impact": [
    {"resource_type": "pod|node|deployment|service", "resource_name": "名称", "namespace": "命名空间", "impact_level": "critical|high|medium|low"}
  ]
}
` + "```" + `

注意事项：
- 如果上下文信息不足，明确说明需要补充哪些信息
- 不要编造不存在的告警或指标
- 建议要具体、可操作，不要泛泛而谈
- 涉及危险操作（删除、重启、扩容）时，提醒用户确认后再执行
- recommendations 中的 restart/scale/rollback 等高风险操作必须标注 risk，并包含 target、namespace、parameters
- 必须在回答末尾包含完整的 JSON 代码块，这是系统解析结构化数据的唯一来源`
}
