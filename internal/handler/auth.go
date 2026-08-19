package handler

import (
	"strconv"

	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AuthHandler 处理认证请求。
type AuthHandler struct {
	AuthService *auth.Service
	UserRepo    *auth.Repository
}

// Login 处理 POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	if h.AuthService == nil {
		response.Internal(c, "认证服务未初始化")
		return
	}

	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	result, err := h.AuthService.Login(c.Request.Context(), req)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.OK(c, result)
}

// Logout 处理 POST /api/v1/auth/logout
// JWT 是无状态的，登出由前端删除 token 完成。
func (h *AuthHandler) Logout(c *gin.Context) {
	response.OK(c, gin.H{"message": "登出成功"})
}

// Me 处理 GET /api/v1/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	// 优先从 context 取（如果经过了 AuthMiddleware）
	user := auth.CurrentUser(c)
	if user != nil {
		response.OK(c, user)
		return
	}
	// 否则自己从 Authorization header 解析 token
	if h.AuthService == nil {
		response.Unauthorized(c, "认证服务未初始化")
		return
	}
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		response.Unauthorized(c, "未认证")
		return
	}
	token := authHeader[7:]
	user, err := h.AuthService.ValidateToken(c.Request.Context(), token)
	if err != nil {
		response.Unauthorized(c, "Token 无效或已过期")
		return
	}
	response.OK(c, user)
}

// ListUsers 处理 GET /api/v1/users?page=&page_size=
func (h *AuthHandler) ListUsers(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	users, total, err := h.UserRepo.List(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Internal(c, "查询用户失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{"users": users, "total": total, "page": page, "page_size": pageSize})
}

// CreateUser 处理 POST /api/v1/users
func (h *AuthHandler) CreateUser(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Email    string `json:"email"`
		FullName string `json:"full_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Internal(c, "密码加密失败: "+err.Error())
		return
	}

	user := &auth.User{
		Username:     req.Username,
		PasswordHash: hash,
		Email:        req.Email,
		FullName:     req.FullName,
		Status:       "active",
	}

	if err := h.UserRepo.Create(c.Request.Context(), user); err != nil {
		response.Internal(c, "创建用户失败: "+err.Error())
		return
	}

	response.Created(c, user)
}

// ListRoles 处理 GET /api/v1/roles
func (h *AuthHandler) ListRoles(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	roles, err := h.UserRepo.ListRoles(c.Request.Context())
	if err != nil {
		response.Internal(c, "查询角色失败: "+err.Error())
		return
	}

	response.OK(c, roles)
}
