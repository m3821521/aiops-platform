package handler

import (
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// RCAHandler 处理根因分析请求。
// P1-X.10 Phase 6: Legacy API 已迁移到统一 Pipeline，不再使用 Legacy Engine。
type RCAHandler struct {
	AlertRepo *alert.Repository
	Pipeline  *rca.Pipeline
}

// Analyze 处理 GET /api/v1/rca/analyze?start=<rfc3339>&end=<rfc3339>
// 查询指定时间范围内的 firing 告警，进行根因分析。
// P1-X.10 Phase 6: 统一使用 Pipeline 进行 RCA 计算，消除两套生产 RCA 逻辑。
func (h *RCAHandler) Analyze(c *gin.Context) {
	if h.Pipeline == nil || h.AlertRepo == nil {
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

	// 按时间范围过滤。
	var firingAlerts []alert.Alert
	for _, a := range alerts {
		if a.StartsAt.Before(start) || a.StartsAt.After(end) {
			continue
		}
		firingAlerts = append(firingAlerts, a)
	}

	// 没有 firing alerts，返回空结果（Empty != Error）。
	if len(firingAlerts) == 0 {
		response.OK(c, rca.RCAResult{
			Status:          rca.RCAStatusInsufficientEvidence,
			RootCause:       "",
			Confidence:      0,
			RootCauseStatus: rca.RootCauseStatusUnknown,
			Explanation:     "当前时间范围内没有 firing 告警，无法进行根因分析。",
			GeneratedAt:     time.Now(),
		})
		return
	}

	// P1-X.10 Phase 6: 使用第一个 firing alert 的资源信息作为主要分析对象。
	// 统一调用 Pipeline，消除 Legacy Engine 与 Pipeline 两套标准。
	primary := firingAlerts[0]
	// 从 Labels 中提取 cluster，没有则使用默认值。
	cluster := "k8ss"
	if primary.Labels != nil {
		if c, ok := primary.Labels["cluster"]; ok && c != "" {
			cluster = c
		}
	}
	resourceType := "service"
	resourceName := primary.Service
	if primary.Pod != "" {
		resourceType = "pod"
		resourceName = primary.Pod
	} else if primary.Node != "" {
		resourceType = "node"
		resourceName = primary.Node
	}

	// 使用临时 incidentID=0（Legacy API 不关联具体 incident）。
	result, err := h.Pipeline.Analyze(
		c.Request.Context(),
		0, // legacy API 不关联 incident
		cluster,
		primary.Namespace,
		primary.Service,
		resourceType,
		resourceName,
		start,
		end,
	)
	if err != nil {
		response.Internal(c, "RCA 分析失败: "+err.Error())
		return
	}

	response.OK(c, result)
}
