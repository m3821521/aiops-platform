package ai

import (
	"strconv"

	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// IncidentAIHandler 处理基于 Incident 的 AI 分析请求。
type IncidentAIHandler struct {
	Service    *AnalysisService
	Repository *AIAnalysisRepository
	Enabled    bool
}

// Analyze 处理 POST /api/v1/incidents/:id/ai-analyze
func (h *IncidentAIHandler) Analyze(c *gin.Context) {
	if !h.Enabled || h.Service == nil {
		response.BadRequest(c, "AI 服务未启用")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 incident id")
		return
	}

	result, err := h.Service.AnalyzeIncident(c.Request.Context(), id)
	if err != nil {
		response.Internal(c, "AI 分析失败: "+err.Error())
		return
	}

	// 保存快照。
	if h.Repository != nil {
		record := &AIAnalysisRecord{
			IncidentID: id,
			Result:     result,
		}
		if _, err := h.Repository.Create(c.Request.Context(), record); err != nil {
			// 保存失败不影响返回。
		}
	}

	response.OK(c, result)
}

// GetLatest 处理 GET /api/v1/incidents/:id/ai-analysis
func (h *IncidentAIHandler) GetLatest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 incident id")
		return
	}
	if h.Repository == nil {
		response.NotFound(c, "AI 分析存储未配置")
		return
	}
	record, err := h.Repository.FindLatest(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "未找到 AI 分析结果")
		return
	}
	response.OK(c, record.Result)
}

// GetHistory 处理 GET /api/v1/incidents/:id/ai-analysis/history
func (h *IncidentAIHandler) GetHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 incident id")
		return
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if h.Repository == nil {
		response.NotFound(c, "AI 分析存储未配置")
		return
	}
	records, err := h.Repository.FindHistory(c.Request.Context(), id, limit)
	if err != nil {
		response.Internal(c, "查询 AI 分析历史失败: "+err.Error())
		return
	}
	response.OK(c, records)
}
