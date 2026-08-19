package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

// requestIDKey 是 context 中存储 request_id 的 key。
type requestIDKey struct{}

// RequestID 为每个请求生成或透传 X-Request-ID。
// - 如果客户端提供了 X-Request-ID，直接使用（便于链路追踪）。
// - 否则生成 16 字节随机 hex 字符串。
// request_id 会写入：
//   1. gin context（c.Set("request_id", id)）
//   2. 响应头 X-Request-ID
//   3. context（通过 c.Request.WithContext），供下游 service 使用
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)

		// 注入到 request context，供不直接持有 gin.Context 的下游使用。
		ctx := context.WithValue(c.Request.Context(), requestIDKey{}, id)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// GetRequestID 从 context 中提取 request_id。
// 适用于 service/repository 层（持有 context.Context 而非 gin.Context）。
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// generateRequestID 生成 16 字节随机 hex 字符串（32 字符）。
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下 fallback 到时间戳 hex，保证非空。
		return hex.EncodeToString([]byte("fallback"))
	}
	return hex.EncodeToString(b)
}

// 确保 http 包被引用（未来可能扩展）。
var _ = http.StatusOK
