package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// AIContextProvider 是 AI 上下文提供者接口。
// 通过接口解耦，避免 ai → 各业务包的循环依赖。
type AIContextProvider interface {
	BuildContext(ctx context.Context, incidentID int64) (*AIContext, error)
}

// AnalysisService 是 AI 分析服务。
type AnalysisService struct {
	provider     Provider
	contextProv  AIContextProvider
	timeout      time.Duration
}

// NewAnalysisService 创建 AI 分析服务。
func NewAnalysisService(provider Provider, contextProv AIContextProvider, timeoutSec int) *AnalysisService {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &AnalysisService{
		provider:    provider,
		contextProv: contextProv,
		timeout:     time.Duration(timeoutSec) * time.Second,
	}
}

// AnalyzeIncident 对指定 Incident 执行 AI 分析。
func (s *AnalysisService) AnalyzeIncident(ctx context.Context, incidentID int64) (*AIAnalysisResult, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("AI 服务未配置")
	}
	if s.contextProv == nil {
		return nil, fmt.Errorf("AI 上下文提供者未配置")
	}

	// 1. 构建 AIContext。
	aiCtx, err := s.contextProv.BuildContext(ctx, incidentID)
	if err != nil {
		return nil, fmt.Errorf("构建 AI 上下文失败: %w", err)
	}

	// 2. 构建 Prompt。
	messages, err := BuildMessages(*aiCtx)
	if err != nil {
		return nil, fmt.Errorf("构建 Prompt 失败: %w", err)
	}

	// 3. 调用 LLM（带超时）。
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	raw, err := s.provider.Chat(callCtx, messages)
	if err != nil {
		return nil, fmt.Errorf("调用 AI 模型失败: %w", err)
	}

	// 4. 解析和验证结果。
	result, err := ParseAIAnalysisResult(raw)
	if err != nil {
		slog.Warn("ai: parse result failed", "err", err, "raw", truncate(raw, 500))
		return nil, fmt.Errorf("AI 输出解析失败: %w", err)
	}

	// 5. 验证 Evidence References（防止 AI 幻觉引用不存在的 Evidence）。
	validIDs := CollectEvidenceIDs(*aiCtx)
	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	if len(rejected) > 0 {
		slog.Warn("ai: invalid evidence references rejected", "rejected", rejected, "incident_id", incidentID)
	}
	result.Evidence = accepted

	// 6. Confidence 校验（clamp 到 0~1）。
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}

	// Case A: 完全没有有效 Evidence (validIDs == 0 && accepted == 0)
	// → confidence = 0，Root Cause 必须表达证据不足
	if len(validIDs) == 0 && len(result.Evidence) == 0 {
		result.Confidence = 0
		if result.RootCauseExplanation == "" || !containsInsufficientEvidence(result.RootCauseExplanation) {
			result.RootCauseExplanation = "当前没有收集到任何有效证据（告警、异常、指标、日志、事件、拓扑），无法确定具体根因。请检查数据源连接是否正常。"
		}
		if result.Summary == "" || !containsInsufficientEvidence(result.Summary) {
			result.Summary = "证据不足，无法确定根因。"
		}
		slog.Info("ai: no valid evidence available, confidence set to 0", "incident_id", incidentID)
	} else if len(result.Evidence) == 0 && len(validIDs) > 0 {
		// Case B: 有 Evidence 但 AI 没有引用任何有效 ID → confidence <= 0.3
		if result.Confidence > 0.3 {
			result.Confidence = 0.3
		}
		slog.Info("ai: evidence exists but no valid references, confidence capped at 0.3", "incident_id", incidentID)
	}

	// 7. 补充元数据。
	result.DataSources = aiCtx.DataSources
	result.GeneratedAt = time.Now()
	result.Model = s.provider.Name()

	return result, nil
}

// IsEnabled 检查 AI 服务是否启用。
func (s *AnalysisService) IsEnabled() bool {
	return s != nil && s.provider != nil
}

// containsInsufficientEvidence 检查文本是否已经表达了"证据不足"的语义。
func containsInsufficientEvidence(text string) bool {
	lower := strings.ToLower(text)
	keywords := []string{
		"insufficient evidence",
		"unable to determine root cause",
		"not enough evidence",
		"证据不足",
		"无法确定",
		"无法确定根因",
		"无法确定具体",
		"缺少证据",
		"没有足够",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
