package handler

import (
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// RCAHandler 处理根因分析请求。
type RCAHandler struct {
	AlertRepo *alert.Repository
	Engine    *rca.Engine
}

// Analyze 处理 GET /api/v1/rca/analyze?start=<rfc3339>&end=<rfc3339>
// 查询指定时间范围内的 firing 告警，进行根因分析。
func (h *RCAHandler) Analyze(c *gin.Context) {
	if h.Engine == nil || h.AlertRepo == nil {
		response.Internal(c, "RCA 服务未初始化")
		return
	}

	// 时间范围，默认最近 1 小时。
	end := time.Now()
	start := end.Add(-1 * time.Hour)

	if s := c.Query("start"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			response.BadRequest(c, "start 格式错误，需 RFC3339")
			return
		}
		start = parsed
	}
	if e := c.Query("end"); e != "" {
		parsed, err := time.Parse(time.RFC3339, e)
		if err != nil {
			response.BadRequest(c, "end 格式错误，需 RFC3339")
			return
		}
		end = parsed
	}

	// 查询所有 firing 告警（不分页，RCA 需要全量）。
	alerts, _, err := h.AlertRepo.List(c.Request.Context(), alert.ListFilter{
		Status: alert.StatusFiring,
	}, 1, 10000)
	if err != nil {
		response.Internal(c, "查询告警失败: "+err.Error())
		return
	}

	// 按时间范围过滤，并转换为 RCA 输入格式。
	var inputs []rca.AlertInfo
	for _, a := range alerts {
		if a.StartsAt.Before(start) || a.StartsAt.After(end) {
			continue
		}
		inputs = append(inputs, rca.AlertInfo{
			ID:          a.ID,
			Fingerprint: a.Fingerprint,
			Alertname:   a.Alertname,
			Severity:    a.Severity,
			Service:     a.Service,
			Namespace:   a.Namespace,
			Pod:         a.Pod,
			Node:        a.Node,
			StartsAt:    a.StartsAt,
			Labels:      a.Labels,
		})
	}

	result := h.Engine.Analyze(inputs)
	response.OK(c, result)
}
