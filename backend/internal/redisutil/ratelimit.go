package redisutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiter 基于 Redis 的 API 限流器。
// 使用固定窗口算法：每个时间窗口内最多允许 limit 次请求。
type RateLimiter struct {
	client *redis.Client
	limit  int           // 每个窗口最大请求数
	window time.Duration // 窗口大小
}

// NewRateLimiter 创建限流器。
func NewRateLimiter(client *redis.Client, limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 100
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

// Allow 检查是否允许请求。
// 返回是否允许、当前窗口剩余请求数、重置时间。
func (rl *RateLimiter) Allow(ctx context.Context, identifier string) (bool, int, time.Time, error) {
	if rl.client == nil {
		return true, rl.limit, time.Now().Add(rl.window), nil
	}

	key := fmt.Sprintf("ratelimit:%s:%d", identifier, time.Now().Unix()/int64(rl.window.Seconds()))

	// 原子操作：INCR + 首次设置过期时间。
	pipe := rl.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rl.window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		// Redis 故障时放行，避免限流服务不可用导致整个系统不可用。
		return true, rl.limit, time.Now().Add(rl.window), nil
	}

	count := incr.Val()
	resetTime := time.Now().Add(rl.window)
	remaining := rl.limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	if count > int64(rl.limit) {
		return false, remaining, resetTime, nil
	}
	return true, remaining, resetTime, nil
}

// Middleware Gin 限流中间件。
// 按 IP 限流。
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := c.ClientIP()
		allowed, remaining, resetTime, err := rl.Allow(c.Request.Context(), identifier)
		if err != nil {
			// Redis 故障时放行。
			c.Next()
			return
		}

		// 设置限流响应头。
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "请求过于频繁，请稍后再试",
				"limit":       rl.limit,
				"reset_at":    resetTime,
				"retry_after": int(resetTime.Sub(time.Now()).Seconds()),
			})
			return
		}
		c.Next()
	}
}

// ErrRateLimited 是限流错误。
var ErrRateLimited = errors.New("rate limited")
