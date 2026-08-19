package handler

import (
	"fmt"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/ai/tools"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AIHandler 处理 AI 助手请求。
type AIHandler struct {
	Assistant *ai.Assistant
	Engine    *tools.Engine
	AuditRepo *tools.ToolAuditRepository
	Enabled   bool
}

// AskRequest 是 AI 问答的请求体。
type AskRequest struct {
	Question   string `json:"question" binding:"required"`
	IncidentID int64  `json:"incident_id,omitempty"`
	Service    string `json:"service,omitempty"`
	Duration   string `json:"duration,omitempty"`
}

// AskResponse 是 AI 问答的响应。
type AskResponse struct {
	Answer      string                  `json:"answer"`
	Summary     string                  `json:"summary,omitempty"`
	RootCause   string                  `json:"root_cause,omitempty"`
	Confidence  float64                 `json:"confidence,omitempty"`
	Evidence    []tools.AgentEvidence   `json:"evidence,omitempty"`
	Recommendations []tools.AgentRecommendation `json:"recommendations,omitempty"`
	ToolCalls   []tools.ToolCall        `json:"tool_calls,omitempty"`
	DurationMs  int64                   `json:"duration_ms,omitempty"`
}

// Ask 处理 POST /api/v1/ai/ask
// Body: {"question": "...", "incident_id": 7}
func (h *AIHandler) Ask(c *gin.Context) {
	if !h.Enabled || h.Assistant == nil {
		response.Internal(c, "AI 助手未启用，请在配置中设置 ai.enabled=true")
		return
	}

	var req AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	// 如果有 incident_id 且 Engine 可用，使用 Tool Calling Engine。
	if req.IncidentID > 0 && h.Engine != nil {
		incidentContext := fmt.Sprintf("Incident ID: %d", req.IncidentID)
		result, err := h.Engine.Ask(c.Request.Context(), req.Question, incidentContext)
		if err != nil {
			response.Internal(c, "AI 分析失败: "+err.Error())
			return
		}

		// 记录 Tool 审计。
		if h.AuditRepo != nil {
			requestID := c.GetString("request_id")
			userID, _ := c.Get("user_id")
			uid, _ := userID.(int64)
			for _, call := range result.ToolCalls {
				record := tools.RecordFromToolCall(call, requestID, req.IncidentID, uid)
				_ = h.AuditRepo.Create(c.Request.Context(), record)
			}
		}

		response.OK(c, AskResponse{
			Answer:         result.Response.Answer,
			Summary:        result.Response.Summary,
			RootCause:      result.Response.RootCause,
			Confidence:     result.Response.Confidence,
			Evidence:       result.Response.Evidence,
			Recommendations: result.Response.Recommendations,
			ToolCalls:      result.ToolCalls,
			DurationMs:     result.Duration.Milliseconds(),
		})
		return
	}

	// 否则使用传统 Assistant。
	aiReq := ai.AskRequest{
		Question: req.Question,
		Service:  req.Service,
		Duration: req.Duration,
	}
	result, err := h.Assistant.Ask(c.Request.Context(), aiReq)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, result)
}

// ListAudit 处理 GET /api/v1/ai/audit
func (h *AIHandler) ListAudit(c *gin.Context) {
	if h.AuditRepo == nil {
		response.Internal(c, "AI 审计未配置")
		return
	}

	filter := tools.ToolAuditFilter{}
	if id := c.Query("incident_id"); id != "" {
		fmt.Sscanf(id, "%d", &filter.IncidentID)
	}
	if tn := c.Query("tool_name"); tn != "" {
		filter.ToolName = tn
	}

	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if ps := c.Query("page_size"); ps != "" {
		fmt.Sscanf(ps, "%d", &pageSize)
	}

	records, total, err := h.AuditRepo.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, "查询审计日志失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"items": records, "total": total, "page": page, "page_size": pageSize})
}
