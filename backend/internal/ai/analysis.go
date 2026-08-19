package ai

import (
	"context"
	"fmt"
	"log/slog"
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

	// 5. 补充元数据。
	result.DataSources = aiCtx.DataSources
	result.GeneratedAt = time.Now()
	result.Model = s.provider.Name()

	return result, nil
}

// IsEnabled 检查 AI 服务是否启用。
func (s *AnalysisService) IsEnabled() bool {
	return s != nil && s.provider != nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
