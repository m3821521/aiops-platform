package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AlertHandler 处理告警相关的 HTTP 请求。
type AlertHandler struct {
	Repo            *alert.Repository
	Aggregator      *alert.Aggregator
	NoiseReducer    *alert.NoiseReducer
	IncidentService *incident.Service // 可选：告警自动关联 Incident
}

// listResult 是分页查询的返回结构。
type listResult struct {
	Items    []alert.Alert `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// ReceiveWebhook 处理 POST /api/v1/alerts/webhook
// 接收 Alertmanager 推送的告警，按 fingerprint upsert 到数据库。
func (h *AlertHandler) ReceiveWebhook(c *gin.Context) {
	var payload alert.WebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}
	if len(payload.Alerts) == 0 {
		response.BadRequest(c, "alerts 不能为空")
		return
	}

	created := 0
	updated := 0
	for _, wa := range payload.Alerts {
		a := wa.ToAlert()
		if a.Fingerprint == "" {
			continue
		}
		saved, err := h.Repo.Upsert(c.Request.Context(), &a)
		if err != nil {
			response.Internal(c, "保存告警失败: "+err.Error())
			return
		}
		if saved.CreatedAt.Equal(saved.UpdatedAt) {
			created++
		} else {
			updated++
		}

		// 关联到 Incident（可选，失败不影响告警保存）。
		if h.IncidentService != nil {
			sig := alertToSignal(saved, payload.Status)
			if _, _, err := h.IncidentService.IngestSignal(c.Request.Context(), sig); err != nil {
				slog.Warn("alert: incident correlation failed", "fingerprint", saved.Fingerprint, "err", err)
			}
		}
	}

	response.OK(c, gin.H{"received": len(payload.Alerts), "created": created, "updated": updated})
}

// alertToSignal 将 Alert 转换为统一 Signal 结构。
func alertToSignal(a *alert.Alert, webhookStatus string) incident.Signal {
	resolved := webhookStatus == "resolved" || a.Status == alert.StatusResolved
	resourceType := incident.ResourcePod
	resourceName := a.Pod
	if a.Pod == "" && a.Node != "" {
		resourceType = incident.ResourceNode
		resourceName = a.Node
	}
	if a.Pod == "" && a.Node == "" && a.Service != "" {
		resourceType = incident.ResourceService
		resourceName = a.Service
	}

	// 从 Alert Labels 动态提取 cluster，优先级：cluster > cluster_name
	cluster := ""
	if a.Labels != nil {
		if v, ok := a.Labels["cluster"]; ok && v != "" {
			cluster = v
		} else if v, ok := a.Labels["cluster_name"]; ok && v != "" {
			cluster = v
		}
	}

	return incident.Signal{
		SignalType:   incident.SignalAlert,
		SignalID:     a.Fingerprint,
		Title:        a.Alertname,
		Severity:     a.Severity,
		Cluster:      cluster,
		Namespace:    a.Namespace,
		Service:      a.Service,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Timestamp:    a.StartsAt,
		Resolved:     resolved,
		Labels:       a.Labels,
		Metadata: map[string]any{
			"instance":    a.Instance,
			"annotations": a.Annotations,
		},
	}
}

// List 处理 GET /api/v1/alerts
// 支持过滤参数：status、severity、alertname、namespace、service
// 分页参数：page（默认1）、page_size（默认20，最大100）
func (h *AlertHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := alert.ListFilter{
		Status:    c.Query("status"),
		Severity:  c.Query("severity"),
		Alertname: c.Query("alertname"),
		Namespace: c.Query("namespace"),
		Service:   c.Query("service"),
	}

	items, total, err := h.Repo.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}

	response.OK(c, listResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ListWithProvenance 同 List，但返回带 meta.provenance 的响应。
// sourceUpdatedAt = max(alert.updated_at)，真实来自 MySQL。
func (h *AlertHandler) ListWithProvenance(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := alert.ListFilter{
		Status:    c.Query("status"),
		Severity:  c.Query("severity"),
		Alertname: c.Query("alertname"),
		Namespace: c.Query("namespace"),
		Service:   c.Query("service"),
	}

	items, total, err := h.Repo.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}

	var sourceUpdatedAt time.Time
	hasUpdated := false
	for i := range items {
		if !hasUpdated || items[i].UpdatedAt.After(sourceUpdatedAt) {
			sourceUpdatedAt = items[i].UpdatedAt
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
	response.OKWithProvenance(c, listResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, prov)
}

// Get 处理 GET /api/v1/alerts/:id
func (h *AlertHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 参数无效")
		return
	}

	a, err := h.Repo.FindByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "告警不存在")
			return
		}
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, a)
}

// Acknowledge 处理 POST /api/v1/alerts/:id/acknowledge
// 将告警状态改为 acknowledged。
func (h *AlertHandler) Acknowledge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 参数无效")
		return
	}

	if _, err := h.Repo.FindByID(c.Request.Context(), id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "告警不存在")
			return
		}
		response.Internal(c, err.Error())
		return
	}

	if err := h.Repo.Acknowledge(c.Request.Context(), id); err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, gin.H{"id": id, "status": alert.StatusAcknowledged})
}

// Resolve 处理 POST /api/v1/alerts/:id/resolve
// 将告警状态改为 resolved 并填写结束时间。
func (h *AlertHandler) Resolve(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 参数无效")
		return
	}

	if _, err := h.Repo.FindByID(c.Request.Context(), id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "告警不存在")
			return
		}
		response.Internal(c, err.Error())
		return
	}

	if err := h.Repo.Resolve(c.Request.Context(), id, time.Now()); err != nil {
		response.Internal(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, response.Body{Code: 0, Message: "success", Data: gin.H{"id": id, "status": alert.StatusResolved}})
}

// Aggregate 处理 GET /api/v1/alerts/aggregate?dimension=service
// 将 firing 状态的告警按维度（service/node/namespace）聚合，返回告警组列表。
func (h *AlertHandler) Aggregate(c *gin.Context) {
	if h.Aggregator == nil {
		response.Internal(c, "聚合引擎未初始化")
		return
	}

	dimension := c.DefaultQuery("dimension", alert.DimByService)
	if dimension != alert.DimByService && dimension != alert.DimByNode && dimension != alert.DimByNamespace {
		response.BadRequest(c, "dimension 取值只能是 service、node、namespace")
		return
	}

	groups, err := h.Aggregator.Aggregate(c.Request.Context(), dimension)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, groups)
}

// Reduce 处理 GET /api/v1/alerts/noise?dimension=service
// 对最近时间窗口内的告警进行去重、分组，并检测告警风暴。
func (h *AlertHandler) Reduce(c *gin.Context) {
	if h.NoiseReducer == nil {
		response.Internal(c, "降噪引擎未初始化")
		return
	}

	dimension := c.DefaultQuery("dimension", alert.DimByService)
	if dimension != alert.DimByService && dimension != alert.DimByNode && dimension != alert.DimByNamespace {
		response.BadRequest(c, "dimension 取值只能是 service、node、namespace")
		return
	}

	result, err := h.NoiseReducer.Reduce(c.Request.Context(), dimension)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, result)
}
