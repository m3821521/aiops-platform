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
	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// aiAskTimeout 是 AI ask 请求的整体超时。
// P2-AI-ASSISTANT-PERF-002 Phase 1: 从 25s 调整为 60s。
// 原因：DeepSeek 等 Provider 对简单问题的实际响应时间约 23~25s，
// 25s overall timeout 与正常 Provider latency 过于贴近，导致误超时。
// 60s 为合理的上层边界，Provider HTTP timeout (60s) 与之对齐。
const aiAskTimeout = 60 * time.Second

// AIErrorType 是 AI 请求错误的分类类型，用于可观测性和前端错误展示。
type AIErrorType string

const (
	// AIErrorTimeout 表示 Backend overall deadline 到期（60s）。
	AIErrorTimeout AIErrorType = "AI_TIMEOUT"
	// AIErrorProviderTimeout 表示 Provider HTTP request 自身 timeout（非 overall context）。
	AIErrorProviderTimeout AIErrorType = "AI_PROVIDER_TIMEOUT"
	// AIErrorClientCancelled 表示客户端主动断开（context.Canceled）。
	AIErrorClientCancelled AIErrorType = "AI_CLIENT_CANCELLED"
	// AIErrorProviderError 表示 Provider 返回 HTTP 错误（401/429/500 等）。
	AIErrorProviderError AIErrorType = "AI_PROVIDER_ERROR"
	// AIErrorToolError 表示 Tool execution failure。
	AIErrorToolError AIErrorType = "AI_TOOL_ERROR"
	// AIErrorUnknown 表示未分类的普通错误。
	AIErrorUnknown AIErrorType = "AI_UNKNOWN_ERROR"
)

// classifyAIError 根据错误来源和 context 状态对 AI 错误进行分类。
func classifyAIError(err error, askCtx context.Context) AIErrorType {
	if err == nil {
		return AIErrorUnknown
	}
	errMsg := err.Error()

	// 1. Client cancelled：优先判断，避免被误分类为 timeout。
	if errors.Is(err, tools.ErrRequestCanceled) || askCtx.Err() == context.Canceled {
		return AIErrorClientCancelled
	}
	// context.Canceled 也可能被包装在 provider error 中。
	if strings.Contains(errMsg, "context canceled") {
		return AIErrorClientCancelled
	}

	// 2. Overall timeout：Backend overall deadline 到期。
	if errors.Is(err, tools.ErrRequestTimeout) || askCtx.Err() == context.DeadlineExceeded {
		return AIErrorTimeout
	}
	// context deadline exceeded 也可能被包装在 provider error 中。
	if strings.Contains(errMsg, "context deadline exceeded") {
		return AIErrorTimeout
	}

	// 3. LLM / Provider timeout（非 overall context，而是 provider 自身 timeout）。
	if errors.Is(err, tools.ErrLLMTimeout) {
		return AIErrorProviderTimeout
	}
	// net/http timeout 错误（Client.Timeout 触发）。
	if strings.Contains(errMsg, "Client.Timeout") || strings.Contains(errMsg, "context deadline exceeded") {
		// 注意：这里已经在上面判断过 overall context deadline，
		// 如果走到这里且包含 deadline exceeded，说明是 provider 自身的 context。
		return AIErrorProviderTimeout
	}

	// 4. Tool timeout / error。
	if errors.Is(err, tools.ErrToolTimeout) {
		return AIErrorToolError
	}
	if strings.Contains(errMsg, "tool") && (strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "failed") || strings.Contains(errMsg, "error")) {
		return AIErrorToolError
	}

	// 5. Provider HTTP 错误（401/429/500 等）。
	if strings.Contains(errMsg, "API Key") || strings.Contains(errMsg, "权限") ||
		strings.Contains(errMsg, "频率超限") || strings.Contains(errMsg, "模型不存在") ||
		strings.Contains(errMsg, "AI 服务错误") || strings.Contains(errMsg, "AI 服务返回错误") {
		return AIErrorProviderError
	}

	return AIErrorUnknown
}

// handleAIError 将 AI Engine/Assistant 的错误转换为用户可理解的 HTTP 响应。
// 区分：overall timeout / client canceled / LLM timeout / Tool timeout / Provider error / 普通错误。
// 返回 true 表示已经处理并写入响应，调用方应 return。
func handleAIError(c *gin.Context, err error, askCtx context.Context) bool {
	if err == nil {
		return false
	}

	errType := classifyAIError(err, askCtx)
	requestID := c.GetString("request_id")

	// 根据错误类型记录不同级别的日志。
	switch errType {
	case AIErrorClientCancelled:
		slog.Info("ai: request cancelled by client",
			"request_id", requestID,
			"error_type", errType,
			"err", err.Error())
	case AIErrorTimeout:
		slog.Warn("ai: request timeout",
			"request_id", requestID,
			"error_type", errType,
			"timeout_seconds", int(aiAskTimeout.Seconds()),
			"err", err.Error())
	case AIErrorProviderTimeout:
		slog.Warn("ai: provider timeout",
			"request_id", requestID,
			"error_type", errType,
			"err", err.Error())
	case AIErrorProviderError:
		slog.Warn("ai: provider error",
			"request_id", requestID,
			"error_type", errType,
			"err", err.Error())
	case AIErrorToolError:
		slog.Warn("ai: tool error",
			"request_id", requestID,
			"error_type", errType,
			"err", err.Error())
	default:
		slog.Error("ai: request failed",
			"request_id", requestID,
			"error_type", errType,
			"err", err.Error())
	}

	// 根据错误类型返回对应的 HTTP 响应。
	switch errType {
	case AIErrorClientCancelled:
		// 客户端已断开，使用 499 Client Closed Request 语义。
		c.Status(499)
		return true

	case AIErrorTimeout:
		response.ServiceUnavailable(c, fmt.Sprintf(
			"AI 请求超时（%d秒），问题可能过于复杂或 AI 服务响应较慢。请尝试简化问题或稍后重试。",
			int(aiAskTimeout.Seconds())))
		return true

	case AIErrorProviderTimeout:
		response.ServiceUnavailable(c, "AI 模型响应超时，请稍后重试")
		return true

	case AIErrorProviderError:
		// Provider 错误直接返回错误信息（已在 provider 层脱敏）。
		response.ServiceUnavailable(c, err.Error())
		return true

	case AIErrorToolError:
		response.ServiceUnavailable(c, "AI 工具调用失败: "+err.Error())
		return true

	default:
		// 普通错误：不暴露内部细节和 API Key。
		errMsg := err.Error()
		if strings.Contains(errMsg, "API Key") || strings.Contains(errMsg, "权限") {
			response.BadRequest(c, errMsg)
			return true
		}
		response.ServiceUnavailable(c, "AI 服务暂时不可用: "+errMsg)
		return true
	}
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
	currentUser := auth.CurrentUser(c)
	var uid int64
	if currentUser != nil {
		uid = currentUser.ID
	}

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

	// aiRequestMetrics 记录单次 AI 请求的性能指标，用于 latency instrumentation。
	type aiRequestMetrics struct {
		startTime       time.Time
		totalDurationMs int64
		providerMs      int64
		toolMs          int64
		toolCallCount   int
		status          string // success / timeout / cancelled / error
		errorType       AIErrorType
		hasIncidentID   bool
	}

	metrics := &aiRequestMetrics{
		startTime:     time.Now(),
		status:        "success",
		errorType:     "", // 成功时 error_type 为空
		hasIncidentID: req.IncidentID > 0,
	}

	// defer 记录最终的 AI 请求性能指标。
	defer func() {
		metrics.totalDurationMs = time.Since(metrics.startTime).Milliseconds()
		requestID := c.GetString("request_id")

		// 记录结构化日志，不包含 API Key、完整 Prompt、完整响应。
		slogAttrs := []any{
			"request_id", requestID,
			"total_duration_ms", metrics.totalDurationMs,
			"provider_duration_ms", metrics.providerMs,
			"tool_duration_ms", metrics.toolMs,
			"tool_call_count", metrics.toolCallCount,
			"status", metrics.status,
			"error_type", metrics.errorType,
			"has_incident_id", metrics.hasIncidentID,
			"timeout_seconds", int(aiAskTimeout.Seconds()),
		}

		switch metrics.status {
		case "success":
			slog.Info("ai: request completed", slogAttrs...)
		case "cancelled":
			slog.Info("ai: request cancelled", slogAttrs...)
		case "timeout":
			slog.Warn("ai: request timeout", slogAttrs...)
		default:
			slog.Error("ai: request failed", slogAttrs...)
		}

		// 兼容现有的 RecordAIRequest 指标。
		ai.RecordAIRequest("openai-compatible", "deepseek-v4-flash", metrics.status, float64(metrics.totalDurationMs)/1000.0)
	}()

	// 如果有 incident_id 且 Engine 可用，使用 Tool Calling Engine。
	if req.IncidentID > 0 && h.Engine != nil {
		incidentContext := fmt.Sprintf("Incident ID: %d", req.IncidentID)
		engineStart := time.Now()
		result, err := h.Engine.Ask(askCtx, req.Question, incidentContext)
		if err != nil {
			errType := classifyAIError(err, askCtx)
			metrics.errorType = errType
			switch errType {
			case AIErrorTimeout:
				metrics.status = "timeout"
			case AIErrorClientCancelled:
				metrics.status = "cancelled"
			default:
				metrics.status = "error"
			}
			if handleAIError(c, err, askCtx) {
				return
			}
			response.Internal(c, "AI 分析失败: "+err.Error())
			return
		}

		// 成功：记录 Engine 模式的性能指标。
		metrics.toolCallCount = len(result.ToolCalls)
		metrics.toolMs = result.Duration.Milliseconds()
		// Engine 模式下 provider 耗时 ≈ 总耗时 - tool 耗时 - overhead（近似）。
		if metrics.toolMs > 0 {
			metrics.providerMs = time.Since(engineStart).Milliseconds() - metrics.toolMs
			if metrics.providerMs < 0 {
				metrics.providerMs = 0
			}
		} else {
			metrics.providerMs = time.Since(engineStart).Milliseconds()
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
	providerStart := time.Now()
	result, err := h.Assistant.Ask(askCtx, aiReq)
	if err != nil {
		errType := classifyAIError(err, askCtx)
		metrics.errorType = errType
		switch errType {
		case AIErrorTimeout:
			metrics.status = "timeout"
		case AIErrorClientCancelled:
			metrics.status = "cancelled"
		default:
			metrics.status = "error"
		}
		if handleAIError(c, err, askCtx) {
			return
		}
		response.ServiceUnavailable(c, "AI 服务暂时不可用: "+err.Error())
		return
	}

	// 成功：Assistant 模式没有 Tool 调用，provider 耗时 ≈ 总耗时。
	metrics.toolCallCount = 0
	metrics.providerMs = time.Since(providerStart).Milliseconds()
	metrics.toolMs = 0

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

// sseEvent 是 SSE 事件的通用结构。
type sseEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// writeSSEEvent 向 SSE 连接写入一个事件并立即 Flush。
func writeSSEEvent(c *gin.Context, event string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化 SSE 事件失败: %w", err)
	}
	// SSE 格式: event: xxx\ndata: xxx\n\n
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, string(jsonData)); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// AskStream 处理 POST /api/v1/ai/ask/stream
// 使用 SSE (Server-Sent Events) 实时返回 AI 回复的 token。
// 这是真正的 streaming：从 Provider 收到一个 token 就立即发送给前端，
// 而不是等待完整响应后再拆分。
func (h *AIHandler) AskStream(c *gin.Context) {
	if !h.Enabled || h.Assistant == nil {
		c.JSON(503, gin.H{"error": "AI 服务未启用"})
		return
	}
	if !h.APIKeyConfigured {
		c.JSON(503, gin.H{"error": "AI 服务不可用：API Key 未配置"})
		return
	}

	var req AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}

	requestID := c.GetString("request_id")
	startTime := time.Now()
	var firstTokenTime time.Time
	hasFirstToken := false

	// 设置 SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // 禁用 Nginx buffering
	c.Writer.WriteHeader(200)
	c.Writer.Flush()

	// 发送 start event
	if err := writeSSEEvent(c, "start", gin.H{"request_id": requestID}); err != nil {
		slog.Error("ai: stream start failed", "request_id", requestID, "err", err.Error())
		return
	}

	// AI 请求整体超时（60s，与普通 ask 一致）
	askCtx, cancel := context.WithTimeout(c.Request.Context(), aiAskTimeout)
	defer cancel()

	// Heartbeat ticker：每 10 秒发送一次心跳，防止代理/浏览器 idle timeout
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	// 构建消息
	// 注意：当前 SSE 模式使用简化的消息构建，后续可以复用 Assistant 的 buildMessages 逻辑
	messages := []ai.Message{
		{Role: "system", Content: "你是一个专业的 AIOps 智能运维助手。"},
		{Role: "user", Content: fmt.Sprintf("用户问题：%s\n\n请根据你的运维知识进行分析。", req.Question)},
	}

	// 用于接收 Provider streaming 错误的 channel
	errChan := make(chan error, 1)

	// 启动 Provider streaming
	go func() {
		err := h.Assistant.GetProvider().ChatStream(askCtx, messages, func(chunk ai.StreamChunk) error {
			// 检查 context 是否已取消
			select {
			case <-askCtx.Done():
				return askCtx.Err()
			default:
			}

			if chunk.Done {
				return nil
			}

			// 记录首 token 时间
			if !hasFirstToken {
				firstTokenTime = time.Now()
				hasFirstToken = true
			}

			// 发送 token event
			return writeSSEEvent(c, "token", gin.H{"text": chunk.Text})
		})
		errChan <- err
	}()

	// 等待 streaming 完成或 heartbeat
	var streamErr error
streamLoop:
	for {
		select {
		case err := <-errChan:
			streamErr = err
			break streamLoop
		case <-heartbeatTicker.C:
			// 发送 heartbeat，不包含 AI 内容
			if err := writeSSEEvent(c, "heartbeat", gin.H{}); err != nil {
				slog.Warn("ai: stream heartbeat failed, client disconnected", "request_id", requestID, "err", err.Error())
				// heartbeat 写入失败意味着 client 已断开，立即终止
				cancel()
				streamErr = fmt.Errorf("客户端连接已断开: %w", err)
				break streamLoop
			}
		case <-askCtx.Done():
			// context 取消或超时
			if askCtx.Err() == context.Canceled {
				streamErr = fmt.Errorf("请求已取消: %w", askCtx.Err())
			} else {
				streamErr = fmt.Errorf("请求超时: %w", askCtx.Err())
			}
			break streamLoop
		}
	}

	// 注意：不在这里调用 cancel()，因为 defer cancel() 会在函数返回时执行。
	// 如果在这里提前 cancel()，会导致 classifyAIError 中 askCtx.Err() == context.Canceled，
	// 从而把 Provider error 误分类为 AI_CLIENT_CANCELLED。
	// heartbeat 失败和 askCtx.Done() 的情况已经在各自的 case 中处理了 cancel。

	totalDuration := time.Since(startTime)

	// 处理错误
	if streamErr != nil {
		errType := classifyAIError(streamErr, askCtx)
		errorMsg := streamErr.Error()

		// 脱敏错误信息
		if strings.Contains(errorMsg, "API Key") || strings.Contains(errorMsg, "Authorization") || strings.Contains(errorMsg, "api_key") {
			errorMsg = "AI 服务配置错误，请检查 AI 配置"
		}

		slog.Warn("ai: stream error",
			"request_id", requestID,
			"error_type", errType,
			"total_duration_ms", totalDuration.Milliseconds(),
			"err", streamErr.Error())

		// 发送 error event
		_ = writeSSEEvent(c, "error", gin.H{
			"request_id": requestID,
			"error_type": errType,
			"message":    errorMsg,
		})
		return
	}

	// 计算 TTFT
	var ttftMs int64
	if hasFirstToken {
		ttftMs = firstTokenTime.Sub(startTime).Milliseconds()
	}

	// 记录完成日志
	slog.Info("ai: stream completed",
		"request_id", requestID,
		"total_duration_ms", totalDuration.Milliseconds(),
		"time_to_first_token_ms", ttftMs,
		"streaming", true,
		"has_incident_id", req.IncidentID > 0)

	// 发送 done event
	_ = writeSSEEvent(c, "done", gin.H{
		"request_id":              requestID,
		"total_duration_ms":       totalDuration.Milliseconds(),
		"time_to_first_token_ms":  ttftMs,
		"streaming":               true,
	})
}
