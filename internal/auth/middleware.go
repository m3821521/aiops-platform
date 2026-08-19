package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ContextUserKey 是 context 中存储用户的 key。
const ContextUserKey = "current_user"

// AuthMiddleware JWT 认证中间件。
// 从 Authorization header 提取 Bearer Token，验证后将用户存入 context。
func AuthMiddleware(authService *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "认证格式错误，需 Bearer Token"})
			return
		}

		user, err := authService.ValidateToken(c.Request.Context(), parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期: " + err.Error()})
			return
		}

		c.Set(ContextUserKey, user)
		c.Next()
	}
}

// RequirePermission RBAC 权限检查中间件。
// 需要先通过 AuthMiddleware。
func RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userVal, exists := c.Get(ContextUserKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			return
		}

		user, ok := userVal.(*User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "用户信息无效"})
			return
		}

		if !user.HasPermission(resource, action) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":      "权限不足",
				"required":   resource + ":" + action,
				"user_roles": user.RoleNames(),
			})
			return
		}

		c.Next()
	}
}

// CurrentUser 从 context 中获取当前用户。
func CurrentUser(c *gin.Context) *User {
	if val, exists := c.Get(ContextUserKey); exists {
		if user, ok := val.(*User); ok {
			return user
		}
	}
	return nil
}
