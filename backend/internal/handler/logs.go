package handler

import (
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// LogsHandler 处理日志查询请求。
type LogsHandler struct {
	ES       *logging.Client
	Analyzer *logging.Analyzer
}

// Search 处理 GET /api/v1/logs/search
// 查询参数：keyword, namespace, pod, container, level, trace_id, request_id, start, end, from, size
func (h *LogsHandler) Search(c *gin.Context) {
	if h.ES == nil {
		response.ServiceUnavailable(c, "Elasticsearch 服务未配置")
		return
	}

	q := logging.SearchQuery{
		Keyword:   c.Query("keyword"),
		Namespace: c.Query("namespace"),
		Pod:       c.Query("pod"),
		Container: c.Query("container"),
		Level:     c.Query("level"),
		TraceID:   c.Query("trace_id"),
		RequestID: c.Query("request_id"),
	}

	// 时间范围，默认最近 15 分钟。
	end := time.Now()
	start := end.Add(-15 * time.Minute)
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
	q.Start = start
	q.End = end

	// 分页。
	if from := c.Query("from"); from != "" {
		if v, err := strconv.Atoi(from); err == nil && v >= 0 {
			q.From = v
		}
	}
	if size := c.Query("size"); size != "" {
		if v, err := strconv.Atoi(size); err == nil && v > 0 {
			q.Size = v
		}
	}

	result, err := h.ES.Search(c.Request.Context(), q)
	if err != nil {
		response.ServiceUnavailable(c, "Elasticsearch 服务不可用: "+err.Error())
		return
	}

	response.OK(c, result)
}

// Analyze 处理 GET /api/v1/logs/analyze
// 查询 error/warn 级别日志并进行聚合分析。
func (h *LogsHandler) Analyze(c *gin.Context) {
	if h.ES == nil || h.Analyzer == nil {
		response.Internal(c, "日志分析服务未初始化")
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

	// 查询 error 和 warn 级别日志，最多 5000 条用于分析。
	q := logging.SearchQuery{
		Namespace: c.Query("namespace"),
		Pod:       c.Query("pod"),
		Start:     start,
		End:       end,
		Size:      5000,
	}
	// level 过滤：如果指定了就用指定的，否则查 error+warn。
	if level := c.Query("level"); level != "" {
		q.Level = level
	} else {
		q.Level = "error" // 先查 error，warn 后续可扩展
	}

	result, err := h.ES.Search(c.Request.Context(), q)
	if err != nil {
		response.ServiceUnavailable(c, "Elasticsearch 服务不可用: "+err.Error())
		return
	}

	analysis := h.Analyzer.Analyze(result.Hits)
	response.OK(c, analysis)
}
