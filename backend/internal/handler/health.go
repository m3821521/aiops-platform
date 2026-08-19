package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type HealthHandler struct {
	DB    *gorm.DB
	Redis *redis.Client
}

type healthResult struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Health 存活检查：进程还在就返回 200。
func (h *HealthHandler) Health(c *gin.Context) {
	response.OK(c, gin.H{"status": "ok"})
}

// Ready 就绪检查：MySQL / Redis 都通才算可以接流量。
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{}
	ok := true

	if h.DB != nil {
		sqlDB, err := h.DB.DB()
		if err != nil || sqlDB.PingContext(ctx) != nil {
			checks["mysql"] = "down"
			ok = false
		} else {
			checks["mysql"] = "up"
		}
	} else {
		checks["mysql"] = "not_configured"
		ok = false
	}

	if h.Redis != nil {
		if err := h.Redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "down"
			ok = false
		} else {
			checks["redis"] = "up"
		}
	} else {
		checks["redis"] = "not_configured"
		ok = false
	}

	status := "ok"
	httpStatus := http.StatusOK
	if !ok {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}
	c.JSON(httpStatus, response.Body{
		Code:    httpStatus,
		Message: status,
		Data:    healthResult{Status: status, Checks: checks},
	})
}
