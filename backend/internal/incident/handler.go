package incident

import (
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// Handler 是 Incident 的 HTTP handler。
type Handler struct {
	Service *Service
}

// List GET /api/v1/incidents
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := ListFilter{
		Status:    c.Query("status"),
		Severity:  c.Query("severity"),
		Namespace: c.Query("namespace"),
		Service:   c.Query("service"),
		Cluster:   c.Query("cluster"),
		Keyword:   c.Query("keyword"),
	}
	if start := c.Query("start_time"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			filter.StartTime = &t
		}
	}
	if end := c.Query("end_time"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			filter.EndTime = &t
		}
	}

	incidents, total, err := h.Service.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, "查询 Incident 失败: "+err.Error())
		return
	}

	// 补充 duration 字段。
	type incidentView struct {
		Incident
		Duration string `json:"duration"`
	}
	views := make([]incidentView, 0, len(incidents))
	for i := range incidents {
		views = append(views, incidentView{
			Incident: incidents[i],
			Duration: formatDuration(incidents[i].Duration()),
		})
	}

	response.OK(c, gin.H{
		"items":     views,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ListWithProvenance 同 List，但返回带 meta.provenance 的响应。
// sourceUpdatedAt = max(incident.updated_at)，真实来自 MySQL。
func (h *Handler) ListWithProvenance(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := ListFilter{
		Status:    c.Query("status"),
		Severity:  c.Query("severity"),
		Namespace: c.Query("namespace"),
		Service:   c.Query("service"),
		Cluster:   c.Query("cluster"),
		Keyword:   c.Query("keyword"),
	}
	if start := c.Query("start_time"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			filter.StartTime = &t
		}
	}
	if end := c.Query("end_time"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			filter.EndTime = &t
		}
	}

	incidents, total, err := h.Service.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, "查询 Incident 失败: "+err.Error())
		return
	}

	type incidentView struct {
		Incident
		Duration string `json:"duration"`
	}
	views := make([]incidentView, 0, len(incidents))
	var sourceUpdatedAt time.Time
	hasUpdated := false
	for i := range incidents {
		views = append(views, incidentView{
			Incident: incidents[i],
			Duration: formatDuration(incidents[i].Duration()),
		})
		if !hasUpdated || incidents[i].UpdatedAt.After(sourceUpdatedAt) {
			sourceUpdatedAt = incidents[i].UpdatedAt
			hasUpdated = true
		}
	}

	fetchedAt := time.Now()
	prov := &response.Provenance{
		Source:             "mysql",
		SourceType:         "mysql",
		FetchedAt:          &fetchedAt,
		TimestampAvailable: false,
		TimestampSemantics: "latest_record_updated_at",
	}
	if hasUpdated {
		su := sourceUpdatedAt
		prov.SourceUpdatedAt = &su
	}
	response.OKWithProvenance(c, gin.H{
		"items":     views,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, prov)
}

// Get GET /api/v1/incidents/:id
func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	inc, err := h.Service.Get(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Incident 不存在")
		return
	}
	response.OK(c, inc)
}

// Acknowledge POST /api/v1/incidents/:id/acknowledge
func (h *Handler) Acknowledge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := h.Service.Acknowledge(c.Request.Context(), id); err != nil {
		response.Internal(c, "确认失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"status": StatusAcknowledged})
}

// Resolve POST /api/v1/incidents/:id/resolve
func (h *Handler) Resolve(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := h.Service.Resolve(c.Request.Context(), id); err != nil {
		response.Internal(c, "解决失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"status": StatusResolved})
}

// Close POST /api/v1/incidents/:id/close
func (h *Handler) Close(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := h.Service.Close(c.Request.Context(), id); err != nil {
		response.Internal(c, "关闭失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"status": StatusClosed})
}

// Signals GET /api/v1/incidents/:id/signals
func (h *Handler) Signals(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	signals, err := h.Service.repo.ListSignals(c.Request.Context(), id)
	if err != nil {
		response.Internal(c, "查询信号失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"items": signals, "total": len(signals)})
}

// Timeline GET /api/v1/incidents/:id/timeline
func (h *Handler) Timeline(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	signals, err := h.Service.GetTimeline(c.Request.Context(), id)
	if err != nil {
		response.Internal(c, "查询时间线失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"items": signals, "total": len(signals)})
}

// formatDuration 格式化持续时间为可读字符串。
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	if hours > 0 {
		return strconv.Itoa(hours) + "h" + strconv.Itoa(minutes) + "m"
	}
	if minutes > 0 {
		return strconv.Itoa(minutes) + "m" + strconv.Itoa(seconds) + "s"
	}
	return strconv.Itoa(seconds) + "s"
}
