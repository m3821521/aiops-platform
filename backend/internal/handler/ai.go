package handler

import (
	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AIHandler 处理 AI 助手请求。
type AIHandler struct {
	Assistant *ai.Assistant
	Enabled   bool
}

// Ask 处理 POST /api/v1/ai/ask
// Body: {"question": "...", "service": "...", "duration": "10m"}
func (h *AIHandler) Ask(c *gin.Context) {
	if !h.Enabled || h.Assistant == nil {
		response.Internal(c, "AI 助手未启用，请在配置中设置 ai.enabled=true")
		return
	}

	var req ai.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	result, err := h.Assistant.Ask(c.Request.Context(), req)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}

	response.OK(c, result)
}
