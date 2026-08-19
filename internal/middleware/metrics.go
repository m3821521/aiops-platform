package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP metrics，自动注册到默认 Registry。
// 用 FullPath() 而非 Request.URL.Path，避免路径参数（如 /pods/:name）产生高基数标签。
var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aiops_http_requests_total",
		Help: "Total number of HTTP requests handled.",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aiops_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets, // 0.005 ~ 10s
	}, []string{"method", "path"})

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aiops_http_in_flight_requests",
		Help: "Number of HTTP requests currently being processed.",
	})
)

// Metrics 统计每个请求的数量、延迟、在途数。
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}
