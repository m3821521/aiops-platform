package audit

import (
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
		go repo.Create(c.Request.Context(), log)
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
	go repo.Create(c.Request.Context(), log)
}
