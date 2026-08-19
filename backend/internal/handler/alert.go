package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AlertHandler 处理告警相关的 HTTP 请求。
type AlertHandler struct {
	Repo         *alert.Repository
	Aggregator   *alert.Aggregator
	NoiseReducer *alert.NoiseReducer
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
	}

	response.OK(c, gin.H{"received": len(payload.Alerts), "created": created, "updated": updated})
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
