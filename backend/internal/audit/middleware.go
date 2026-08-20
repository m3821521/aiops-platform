package audit

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/gin-gonic/gin"
)

// Middleware 审计中间件。
// 自动记录所有写操作（POST/PUT/PATCH/DELETE）的审计日志。
func Middleware(repo *Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只记录写操作。
		method := c.Request.Method
		if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
			c.Next()
			return
		}

		// 跳过登录接口（登录有单独的审计）。
		if strings.Contains(c.Request.URL.Path, "/auth/login") {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		// 从 context 获取当前用户。
		user := auth.CurrentUser(c)
		log := &Log{
			Action:    method,
			Resource:  extractResource(c.Request.URL.Path),
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Result:    "success",
			CreatedAt: start,
		}
		if user != nil {
			log.UserID = &user.ID
			log.Username = user.Username
		}
		if c.Writer.Status() >= 400 {
			log.Result = "failed"
		}

		// 异步记录，不阻塞响应。
		// 使用 context.Background() 而不是 c.Request.Context()，
		// 因为请求结束后 context 会被取消，导致异步记录失败。
		go func() {
			// 使用独立的 context，避免请求 context 取消影响
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := repo.Create(ctx, log); err != nil {
				slog.Error("failed to write audit log",
					"action", log.Action,
					"resource", log.Resource,
					"error", err)
			}
		}()
	}
}

// extractResource 从 URL 路径提取资源类型。
// 例如 /api/v1/alerts/webhook → alerts
func extractResource(path string) string {
	parts := strings.Split(path, "/")
	// 路径格式: /api/v1/{resource}/...
	for i, p := range parts {
		if p == "v1" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}

// Record 手动记录一条审计日志。
func Record(repo *Repository, c *gin.Context, action, resource, resourceID string) {
	user := auth.CurrentUser(c)
	log := &Log{
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		Result:     "success",
		CreatedAt:  time.Now(),
	}
	if user != nil {
		log.UserID = &user.ID
		log.Username = user.Username
	}
	// 使用独立的 context，避免请求 context 取消影响
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := repo.Create(ctx, log); err != nil {
			slog.Error("failed to write audit log (Record)",
				"action", log.Action,
				"resource", log.Resource,
				"error", err)
		}
	}()
}
