package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/ai/tools"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// aiAskTimeout 是 AI ask 请求的整体超时。
// 设置为 25s，在 Frontend Axios 30s timeout 之前返回友好错误，
// 避免用户看到 "timeout of 30000ms exceeded" 这种不友好的技术错误。
const aiAskTimeout = 25 * time.Second

// handleAIError 将 AI Engine/Assistant 的错误转换为用户可理解的 HTTP 响应。
// 区分：overall timeout / client canceled / LLM timeout / 普通错误。
// 返回 true 表示已经处理并写入响应，调用方应 return。
func handleAIError(c *gin.Context, err error, askCtx context.Context) bool {
	if err == nil {
		return false
	}

	// 1. Overall request timeout（25s deadline exceeded）
	if errors.Is(err, tools.ErrRequestTimeout) || askCtx.Err() == context.DeadlineExceeded {
		slog.Warn("ai: request timeout", "err", err.Error())
		response.ServiceUnavailable(c, "AI 请求超时（25秒），问题可能过于复杂。请尝试简化问题或缩小范围后重试")
		return true
	}

	// 2. Client canceled（浏览器/Axios 主动断开）
	if errors.Is(err, tools.ErrRequestCanceled) || askCtx.Err() == context.Canceled {
		slog.Info("ai: request canceled by client", "err", err.Error())
		// 客户端已断开，不需要返回完整响应；使用 499 Client Closed Request 语义
		c.Status(499)
		return true
	}

	// 3. LLM timeout（provider 调用超时，但 overall context 可能还有时间）
	if errors.Is(err, tools.ErrLLMTimeout) {
		slog.Warn("ai: LLM timeout", "err", err.Error())
		response.ServiceUnavailable(c, "AI 模型响应超时，请稍后重试")
		return true
	}

	// 4. Tool timeout（单个 Tool 超时，通常 Engine 内部已处理，这里是兜底）
	if errors.Is(err, tools.ErrToolTimeout) {
		slog.Warn("ai: tool timeout", "err", err.Error())
		response.ServiceUnavailable(c, "AI 工具调用超时，请稍后重试")
		return true
	}

	// 5. 普通错误：不暴露内部细节和 API Key
	errMsg := err.Error()
	if strings.Contains(errMsg, "API Key") || strings.Contains(errMsg, "权限") {
		response.BadRequest(c, errMsg)
		return true
	}

	slog.Error("ai: request failed", "err", err)
	response.ServiceUnavailable(c, "AI 服务暂时不可用: "+errMsg)
	return true
}

// AIHandler 处理 AI 助手请求。
type AIHandler struct {
	Assistant       *ai.Assistant
	Engine          *tools.Engine
	AuditRepo       *tools.ToolAuditRepository
	ConversationHdl *ai.ConversationHandler
	Enabled         bool
	APIKeyConfigured bool
}

// UpdateAPIKeyStatus 运行时更新 API Key 配置状态（前端配置后调用）。
func (h *AIHandler) UpdateAPIKeyStatus(configured bool) {
	h.APIKeyConfigured = configured
}

// AskRequest 是 AI 问答的请求体。
type AskRequest struct {
	Question       string `json:"question" binding:"required"`
	IncidentID     int64  `json:"incident_id,omitempty"`
	Service        string `json:"service,omitempty"`
	Duration       string `json:"duration,omitempty"`
	ConversationID int64  `json:"conversation_id,omitempty"`
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
// Body: {"question": "...", "incident_id": 7, "conversation_id": 1}
func (h *AIHandler) Ask(c *gin.Context) {
	if !h.Enabled || h.Assistant == nil {
		response.ServiceUnavailable(c, "AI 服务未启用，请在配置中设置 ai.enabled=true")
		return
	}
	if !h.APIKeyConfigured {
		response.ServiceUnavailable(c, "AI 服务不可用：API Key 未配置。请通过环境变量 AI_API_KEY 或配置文件 ai.api_key 设置")
		return
	}

	var req AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	// AI 请求整体超时：在 Frontend 30s timeout 之前返回友好错误。
	askCtx, cancel := context.WithTimeout(c.Request.Context(), aiAskTimeout)
	defer cancel()

	// 获取用户 ID。
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	// 处理 Conversation。
	var convID int64
	if h.ConversationHdl != nil && uid > 0 {
		if req.ConversationID > 0 {
			// 验证现有对话所有权（防止 IDOR）。
			conv, err := h.ConversationHdl.Repo.GetByIDAndUser(askCtx, req.ConversationID, uid)
			if err != nil || conv.UserID != uid {
				response.Forbidden(c, "无权访问此对话")
				return
			}
			convID = req.ConversationID
		} else {
			// 创建新对话。
			title := req.Question
			if len(title) > 50 {
				title = title[:50]
			}
			var incidentID *int64
			if req.IncidentID > 0 {
				incidentID = &req.IncidentID
			}
			conv, err := h.ConversationHdl.CreateConversation(askCtx, uid, title, incidentID)
			if err == nil {
				convID = conv.ID
			}
		}

		// 保存用户消息。
		if convID > 0 {
			userMsg := &ai.ConversationMessage{
				ConversationID: convID,
				Role:           "user",
				Content:        req.Question,
				CreatedAt:      time.Now(),
			}
			_ = h.ConversationHdl.Repo.AddMessage(askCtx, userMsg)
		}
	}

	// 记录 AI 请求指标。
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		ai.RecordAIRequest("openai", "gpt-4o-mini", "success", duration)
	}()

	// 如果有 incident_id 且 Engine 可用，使用 Tool Calling Engine。
	if req.IncidentID > 0 && h.Engine != nil {
		incidentContext := fmt.Sprintf("Incident ID: %d", req.IncidentID)
		result, err := h.Engine.Ask(askCtx, req.Question, incidentContext)
		if err != nil {
			if handleAIError(c, err, askCtx) {
				return
			}
			response.Internal(c, "AI 分析失败: "+err.Error())
			return
		}

		// 记录 Tool 审计。
		if h.AuditRepo != nil {
			requestID := c.GetString("request_id")
			for _, call := range result.ToolCalls {
				record := tools.RecordFromToolCall(call, requestID, req.IncidentID, uid)
				_ = h.AuditRepo.Create(askCtx, record)
			}
		}

		// 保存助手消息。
		if convID > 0 && h.ConversationHdl != nil {
			evidenceJSON, _ := json.Marshal(result.Response.Evidence)
			recommendJSON, _ := json.Marshal(result.Response.Recommendations)
			toolCallsJSON, _ := json.Marshal(result.ToolCalls)
			assistantMsg := &ai.ConversationMessage{
				ConversationID: convID,
				Role:           "assistant",
				Content:        result.Response.Answer,
				Summary:        result.Response.Summary,
				RootCause:      result.Response.RootCause,
				Confidence:     result.Response.Confidence,
				EvidenceJSON:   string(evidenceJSON),
				RecommendJSON:  string(recommendJSON),
				ToolCallsJSON:  string(toolCallsJSON),
				DurationMs:     result.Duration.Milliseconds(),
				CreatedAt:      time.Now(),
			}
			_ = h.ConversationHdl.Repo.AddMessage(askCtx, assistantMsg)
		}

		response.OK(c, gin.H{
			"answer":          result.Response.Answer,
			"summary":         result.Response.Summary,
			"root_cause":      result.Response.RootCause,
			"confidence":      result.Response.Confidence,
			"evidence":        result.Response.Evidence,
			"recommendations": result.Response.Recommendations,
			"tool_calls":      result.ToolCalls,
			"duration_ms":     result.Duration.Milliseconds(),
			"conversation_id": convID,
		})
		return
	}

	// 否则使用传统 Assistant。
	aiReq := ai.AskRequest{
		Question: req.Question,
		Service:  req.Service,
		Duration: req.Duration,
	}
	result, err := h.Assistant.Ask(askCtx, aiReq)
	if err != nil {
		if handleAIError(c, err, askCtx) {
			return
		}
		response.ServiceUnavailable(c, "AI 服务暂时不可用: "+err.Error())
		return
	}

	// 保存助手消息（传统模式）。
	if convID > 0 && h.ConversationHdl != nil {
		assistantMsg := &ai.ConversationMessage{
			ConversationID: convID,
			Role:           "assistant",
			Content:        result.Answer,
			Summary:        result.Context,
			CreatedAt:      time.Now(),
		}
		_ = h.ConversationHdl.Repo.AddMessage(askCtx, assistantMsg)
	}

	response.OK(c, gin.H{
		"answer":          result.Answer,
		"summary":         result.Summary,
		"root_cause":      result.RootCause,
		"confidence":      result.Confidence,
		"severity":        result.Severity,
		"evidence":        result.Evidence,
		"possible_causes": result.PossibleCauses,
		"recommendations": result.Recommendations,
		"impact":          result.Impact,
		"context":         result.Context,
		"conversation_id": convID,
	})
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
