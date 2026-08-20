package handler

import (
	"regexp"
	"time"

	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// namespaceRe 校验 Kubernetes namespace 格式（DNS-1123 label）。
var namespaceRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// MetricsHandler 处理 Prometheus 指标查询。
type MetricsHandler struct {
	Prom monitoring.Querier
}

// Query 处理 GET /api/v1/metrics/query?query=<promql>&time=<rfc3339>
// query: 必填，PromQL 表达式
// time:  可选，RFC3339 时间点，不传则 Prometheus 使用当前时间
func (h *MetricsHandler) Query(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		response.BadRequest(c, "query 参数不能为空")
		return
	}

	var ts time.Time
	if t := c.Query("time"); t != "" {
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			response.BadRequest(c, "time 参数格式错误，需 RFC3339（如 2026-01-01T00:00:00Z）")
			return
		}
		ts = parsed
	}

	result, err := h.Prom.Query(c.Request.Context(), query, ts)
	if err != nil {
		response.Internal(c, "Prometheus 查询失败: "+err.Error())
		return
	}
	response.OK(c, result)
}

// QueryRange 处理 GET /api/v1/metrics/range?query=<promql>&start=<rfc3339>&end=<rfc3339>&step=<duration>
// query: 必填，PromQL 表达式
// start: 必填，范围起始时间，RFC3339
// end:   必填，范围结束时间，RFC3339
// step:  必填，数据点间隔，如 15s、1m、5m
func (h *MetricsHandler) QueryRange(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		response.BadRequest(c, "query 参数不能为空")
		return
	}

	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		response.BadRequest(c, "start 参数格式错误，需 RFC3339（如 2026-01-01T00:00:00Z）")
		return
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil {
		response.BadRequest(c, "end 参数格式错误，需 RFC3339（如 2026-01-01T00:00:00Z）")
		return
	}
	step, err := time.ParseDuration(c.Query("step"))
	if err != nil {
		response.BadRequest(c, "step 参数格式错误，如 15s、1m、5m")
		return
	}

	result, err := h.Prom.QueryRange(c.Request.Context(), query, start, end, step)
	if err != nil {
		response.Internal(c, "Prometheus 范围查询失败: "+err.Error())
		return
	}
	response.OK(c, result)
}

// ListNodes 处理 GET /api/v1/metrics/nodes
// 返回所有节点的 CPU 和内存使用率。
func (h *MetricsHandler) ListNodes(c *gin.Context) {
	result, err := h.Prom.NodeMetrics(c.Request.Context())
	if err != nil {
		response.OK(c, []any{})
		return
	}
	response.OK(c, result)
}

// ListPods 处理 GET /api/v1/metrics/pods?namespace=<ns>
// 返回 Pod 的 CPU 和内存使用量。namespace 可选，不传则查询所有命名空间。
func (h *MetricsHandler) ListPods(c *gin.Context) {
	namespace := c.Query("namespace")
	// 校验 namespace 格式（DNS-1123 label），防止无效输入。
	if namespace != "" && !namespaceRe.MatchString(namespace) {
		response.BadRequest(c, "namespace 格式不合法，必须符合 DNS-1123 label 格式（小写字母、数字、连字符，且以字母数字开头和结尾）")
		return
	}
	result, err := h.Prom.PodMetrics(c.Request.Context(), namespace)
	if err != nil {
		response.OK(c, []any{})
		return
	}
	response.OK(c, result)
}
