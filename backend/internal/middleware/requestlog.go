package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLog 记录每个 HTTP 请求的结构化日志。
// 包含 method、path、status、cost_ms、request_id、user_id（如果已认证）。
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"cost_ms", time.Since(start).Milliseconds(),
		}

		// request_id（由 RequestID 中间件设置）。
		if rid, exists := c.Get("request_id"); exists {
			attrs = append(attrs, "request_id", rid)
		}

		// user_id（由 AuthMiddleware 设置）。
		if uid, exists := c.Get("user_id"); exists {
			attrs = append(attrs, "user_id", uid)
		}

		// 4xx/5xx 用 Warn 级别，便于告警过滤。
		status := c.Writer.Status()
		if status >= 500 {
			slog.Error("http", attrs...)
		} else if status >= 400 {
			slog.Warn("http", attrs...)
		} else {
			slog.Info("http", attrs...)
		}
	}
}
