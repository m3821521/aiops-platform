package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// ActionCreator 是 Action 创建器接口。
// 通过接口解耦，避免 ai → automation 的循环依赖。
type ActionCreator interface {
	// CreateActionFromRecommendation 从 AI Recommendation 创建 Action。
	// 返回创建的 Action ID，如果创建失败返回 error。
	CreateActionFromRecommendation(ctx context.Context, incidentID int64, userID int64, rec Recommendation) (int64, error)
}

// CreatedAction 记录 AI 自动创建的 Action 信息。
type CreatedAction struct {
	ActionID   int64  `json:"action_id"`
	ActionType string `json:"action_type"`
	Target     string `json:"target"`
	Namespace  string `json:"namespace"`
	Status     string `json:"status"`
}

// autoCreateActionsFromRecommendations 从 AI 分析结果中自动创建可执行 Action。
// 只处理有有效 action_type 的 recommendation，跳过调查/观察类建议。
func autoCreateActionsFromRecommendations(ctx context.Context, creator ActionCreator, incidentID int64, userID int64, recommendations []Recommendation) []CreatedAction {
	if creator == nil || len(recommendations) == 0 {
		return nil
	}

	var created []CreatedAction
	for _, rec := range recommendations {
		// 跳过没有 action_type 或 target 的 recommendation
		if rec.ActionType == "" || rec.Target == "" {
			continue
		}

		actionID, err := creator.CreateActionFromRecommendation(ctx, incidentID, userID, rec)
		if err != nil {
			slog.Warn("ai: auto create action from recommendation failed",
				"incident_id", incidentID,
				"action_type", rec.ActionType,
				"target", rec.Target,
				"error", err,
			)
			continue
		}

		created = append(created, CreatedAction{
			ActionID:   actionID,
			ActionType: rec.ActionType,
			Target:     rec.Target,
			Namespace:  rec.Namespace,
			Status:     "pending_approval",
		})

		slog.Info("ai: auto created action from recommendation",
			"incident_id", incidentID,
			"action_id", actionID,
			"action_type", rec.ActionType,
			"target", rec.Target,
		)
	}

	return created
}

// recommendationToActionParams 将 AI Recommendation 转换为 Action 参数。
// 这是一个辅助函数，供 ActionCreator 实现使用。
func RecommendationToActionParams(rec Recommendation) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	// 复制 AI 提供的 parameters
	if rec.Parameters != nil {
		for k, v := range rec.Parameters {
			params[k] = v
		}
	}

	// 确保有 reason
	if rec.Reason != "" {
		params["reason"] = rec.Reason
	}

	// 确保有 description
	if rec.Description != "" {
		params["description"] = rec.Description
	}

	return params, nil
}

// SerializeParams 将参数序列化为 JSON 字符串，供 Action model 使用。
func SerializeParams(params map[string]interface{}) string {
	if len(params) == 0 {
		return "{}"
	}
	b, err := json.Marshal(params)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// FormatActionReason 格式化 Action 的 reason 字段。
func FormatActionReason(rec Recommendation) string {
	if rec.Reason != "" {
		return rec.Reason
	}
	if rec.Description != "" {
		return rec.Description
	}
	return fmt.Sprintf("AI recommended action: %s", rec.Title)
}
