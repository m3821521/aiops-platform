package handler

import (
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AnomalyHandler 处理异常检测请求。
type AnomalyHandler struct {
	Service *anomaly.Service
}

// DetectRequest 是 API 层的请求结构，step 用字符串便于前端传递。
type detectRequest struct {
	Query      string                  `json:"query"`
	Start      time.Time               `json:"start"`
	End        time.Time               `json:"end"`
	Step       string                  `json:"step"` // 如 "1m", "30s"
	Thresholds anomaly.ThresholdConfig `json:"thresholds"`
}

// Detect 处理 POST /api/v1/anomaly/detect
// 从 Prometheus 拉取指定指标的时间范围数据，用静态阈值检测异常。
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
