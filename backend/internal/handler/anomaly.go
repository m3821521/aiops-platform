package handler

import (
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AnomalyHandler 处理异常检测请求。
type AnomalyHandler struct {
	Service *anomaly.Service
	Repo    *anomaly.Repository
}

// detectRequest 是 API 层的请求结构，step 用字符串便于前端传递。
type detectRequest struct {
	Query      string                  `json:"query"`
	Start      time.Time               `json:"start"`
	End        time.Time               `json:"end"`
	Step       string                  `json:"step"` // 如 "1m", "30s"
	Thresholds anomaly.ThresholdConfig `json:"thresholds"`
}

// Detect 处理 POST /api/v1/anomaly/detect
// 从 Prometheus 拉取指定指标的时间范围数据，用静态阈值检测异常。
// 当 persist=true 时，结果持久化到数据库，严重异常进入 Incident。
func (h *AnomalyHandler) Detect(c *gin.Context) {
	if h.Service == nil {
		response.Internal(c, "异常检测服务未初始化")
		return
	}

	var req detectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	step := time.Minute
	if req.Step != "" {
		parsed, err := time.ParseDuration(req.Step)
		if err != nil {
			response.BadRequest(c, "step 格式错误，如 1m、30s")
			return
		}
		step = parsed
	}

	// persist=true 时执行持久化检测。
	if c.Query("persist") == "true" {
		result, err := h.Service.DetectAndPersist(c.Request.Context(), anomaly.DetectRequest{
			Query:      req.Query,
			Start:      req.Start,
			End:        req.End,
			Step:       step,
			Thresholds: req.Thresholds,
		})
		if err != nil {
			response.Internal(c, err.Error())
			return
		}
		response.OK(c, result)
		return
	}

	result, err := h.Service.Detect(c.Request.Context(), anomaly.DetectRequest{
		Query:      req.Query,
		Start:      req.Start,
		End:        req.End,
		Step:       step,
		Thresholds: req.Thresholds,
	})
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, result)
}

// List 处理 GET /api/v1/anomaly
// 分页查询异常记录，支持多维度筛选。
func (h *AnomalyHandler) List(c *gin.Context) {
	if h.Repo == nil {
		response.Internal(c, "异常存储未初始化")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := anomaly.ListFilter{
		Cluster:      c.Query("cluster"),
		Namespace:    c.Query("namespace"),
		ResourceType: c.Query("resource_type"),
		ResourceName: c.Query("resource_name"),
		Severity:     c.Query("severity"),
		Algorithm:    c.Query("algorithm"),
		Status:       c.Query("status"),
		Metric:       c.Query("metric"),
	}
	if v := c.Query("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.StartTime = &t
		}
	}
	if v := c.Query("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.EndTime = &t
		}
	}

	records, total, err := h.Repo.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}

	response.OK(c, gin.H{
		"items":     records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Get 处理 GET /api/v1/anomaly/:id
// 查询单条异常记录详情。
func (h *AnomalyHandler) Get(c *gin.Context) {
	if h.Repo == nil {
		response.Internal(c, "异常存储未初始化")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}

	rec, err := h.Repo.FindByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "异常记录不存在")
		return
	}
	response.OK(c, rec)
}

// ActiveCount 处理 GET /api/v1/anomaly/active/count
// 返回活跃异常数量（用于 Dashboard 卡片）。
func (h *AnomalyHandler) ActiveCount(c *gin.Context) {
	if h.Repo == nil {
		response.OK(c, gin.H{"count": 0})
		return
	}
	count, err := h.Repo.CountActive(c.Request.Context())
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, gin.H{"count": count})
}
