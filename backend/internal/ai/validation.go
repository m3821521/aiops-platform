package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateAIAnalysisResult 验证 AI 分析结果是否符合 Schema。
func ValidateAIAnalysisResult(result *AIAnalysisResult) error {
	if result == nil {
		return fmt.Errorf("result is nil")
	}
	if strings.TrimSpace(result.Summary) == "" {
		return fmt.Errorf("summary 不能为空")
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence 必须在 0~1 之间，当前: %f", result.Confidence)
	}
	// 验证 Evidence 的 type 合法。
	validTypes := map[string]bool{"alert": true, "anomaly": true, "metric": true, "log": true, "event": true, "topology": true}
	for i, e := range result.Evidence {
		if !validTypes[e.Type] {
			return fmt.Errorf("evidence[%d].type 非法: %s", i, e.Type)
		}
		if strings.TrimSpace(e.Description) == "" {
			return fmt.Errorf("evidence[%d].description 不能为空", i)
		}
	}
	// 验证 Recommendation 的 priority 和 action_type。
	validPriorities := map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
	validActions := map[string]bool{"observe": true, "investigate": true, "restart": true, "scale": true, "rollback": true, "config_change": true, "network_check": true}
	for i, r := range result.Recommendations {
		if !validPriorities[r.Priority] {
			return fmt.Errorf("recommendation[%d].priority 非法: %s", i, r.Priority)
		}
		if !validActions[r.ActionType] {
			return fmt.Errorf("recommendation[%d].action_type 非法: %s", i, r.ActionType)
		}
		if strings.TrimSpace(r.Reason) == "" {
			return fmt.Errorf("recommendation[%d].reason 不能为空", i)
		}
	}
	// 验证 Risk level。
	validRisks := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	for i, r := range result.Risks {
		if !validRisks[r.Level] {
			return fmt.Errorf("risk[%d].level 非法: %s", i, r.Level)
		}
	}
	return nil
}

// ParseAIAnalysisResult 从 LLM 输出中解析结构化结果。
// 支持纯 JSON 和被 Markdown 代码块包裹的 JSON。
func ParseAIAnalysisResult(raw string) (*AIAnalysisResult, error) {
	raw = strings.TrimSpace(raw)
	// 去除 Markdown 代码块标记。
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	// 找到第一个 { 和最后一个 }。
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("未找到有效的 JSON 对象")
	}
	raw = raw[start : end+1]

	var result AIAnalysisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	if err := ValidateAIAnalysisResult(&result); err != nil {
		return nil, fmt.Errorf("结果验证失败: %w", err)
	}
	return &result, nil
}
