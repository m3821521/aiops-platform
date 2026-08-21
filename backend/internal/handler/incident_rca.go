package handler

import (
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// IncidentRCAHandler 处理基于 Incident 的 RCA 请求。
type IncidentRCAHandler struct {
	RCAService     *rca.Service
	IncidentService *incident.Service
}

// Analyze 处理 POST /api/v1/incidents/:id/rca
// 触发 RCA 分析。
func (h *IncidentRCAHandler) Analyze(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 incident id")
		return
	}

	// 获取 Incident 信息。
	inc, err := h.IncidentService.Get(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "incident 不存在")
		return
	}

	endTime := time.Now()
	if inc.EndTime != nil {
		endTime = *inc.EndTime
	}

	// 执行 RCA。
	result, err := h.RCAService.Analyze(
		c.Request.Context(),
		id,
		inc.Cluster,
		inc.Namespace,
		inc.Service,
		inc.ResourceType,
		inc.ResourceName,
		inc.StartTime,
		endTime,
	)
	if err != nil {
		response.Internal(c, "RCA 分析失败: "+err.Error())
		return
	}

	response.OK(c, result)
}

// GetLatest 处理 GET /api/v1/incidents/:id/rca
// 获取最近的 RCA 结果。
func (h *IncidentRCAHandler) GetLatest(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 incident id")
		return
	}

	result, err := h.RCAService.GetLatest(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "未找到 RCA 结果")
		return
	}

	response.OK(c, result)
}

// GetHistory 处理 GET /api/v1/incidents/:id/rca/history
// 获取 RCA 历史。
func (h *IncidentRCAHandler) GetHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
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

	history, err := h.RCAService.GetHistory(c.Request.Context(), id, limit)
	if err != nil {
		response.Internal(c, "查询 RCA 历史失败: "+err.Error())
		return
	}

	response.OK(c, history)
}

// Reanalyze 处理 POST /api/v1/incidents/:id/rca/reanalyze
// 重新执行 RCA 分析。
func (h *IncidentRCAHandler) Reanalyze(c *gin.Context) {
	// 复用 Analyze 逻辑。
	h.Analyze(c)
}

// GetEvidence 处理 GET /api/v1/incidents/:id/evidence
// 获取 Incident 的完整 Evidence 链。独立收集，不依赖 RCA 先执行。
func (h *IncidentRCAHandler) GetEvidence(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 incident id")
		return
	}

	// 获取 Incident 信息。
	inc, err := h.IncidentService.Get(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "incident 不存在")
		return
	}

	endTime := time.Now()
	if inc.EndTime != nil {
		endTime = *inc.EndTime
	}

	// 独立收集 Evidence，不依赖 RCA 结果。
	bundle, err := h.RCAService.CollectEvidence(
		c.Request.Context(),
		id,
		inc.Cluster,
		inc.Namespace,
		inc.Service,
		inc.ResourceType,
		inc.ResourceName,
		inc.StartTime,
		endTime,
	)
	if err != nil {
		response.Internal(c, "收集 Evidence 失败: "+err.Error())
		return
	}

	response.OK(c, bundle)
}
