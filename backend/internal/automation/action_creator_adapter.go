package automation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/incident"
)

// ActionCreatorAdapter 实现 ai.ActionCreator 接口。
// 将 AI Recommendation 转换为 Action 并创建。
type ActionCreatorAdapter struct {
	Service         *Service
	IncidentService *incident.Service
}

// NewActionCreatorAdapter 创建 ActionCreatorAdapter。
func NewActionCreatorAdapter(service *Service, incidentService *incident.Service) *ActionCreatorAdapter {
	return &ActionCreatorAdapter{Service: service, IncidentService: incidentService}
}

// CreateActionFromRecommendation 从 AI Recommendation 创建 Action。
func (a *ActionCreatorAdapter) CreateActionFromRecommendation(ctx context.Context, incidentID int64, userID int64, rec ai.Recommendation) (int64, error) {
	if a.Service == nil {
		return 0, fmt.Errorf("automation service is nil")
	}

	// action_type 白名单校验
	if !IsSupportedActionType(rec.ActionType) {
		return 0, fmt.Errorf("unsupported action type: %s", rec.ActionType)
	}

	// 确定 target_type
	targetType := "deployment"
	if rec.ActionType == "restart_pod" {
		targetType = "pod"
	}

	// 从 Incident 获取 Cluster 和 Namespace（平台上下文，不由 AI 决定）
	cluster := ""
	namespace := rec.Namespace
	if a.IncidentService != nil {
		inc, err := a.IncidentService.Get(ctx, incidentID)
		if err != nil {
			slog.Warn("action_creator: failed to get incident for cluster/namespace", "incident_id", incidentID, "err", err)
		} else {
			if inc.Cluster != "" {
				cluster = inc.Cluster
			}
			if inc.Namespace != "" {
				namespace = inc.Namespace
			}
		}
	}

	// 构建 Action
	action := &Action{
		IncidentID: incidentID,
		ActionType: rec.ActionType,
		TargetType: targetType,
		TargetName: rec.Target,
		Namespace:  namespace,
		Cluster:    cluster,
		Reason:     ai.FormatActionReason(rec),
		Risk:       RiskLevel(rec.Risk),
	}

	// 序列化 parameters
	params, err := ai.RecommendationToActionParams(rec)
	if err != nil {
		return 0, fmt.Errorf("failed to convert recommendation params: %w", err)
	}
	action.Parameters = ai.SerializeParams(params)

	// 创建 Action
	created, err := a.Service.CreateAction(ctx, action, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to create action: %w", err)
	}

	return created.ID, nil
}
