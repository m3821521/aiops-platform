package handler

import (
	"fmt"
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

	response.OK(c, gin.H{"items": users, "users": users, "total": total, "page": page, "page_size": pageSize})
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

// AssignRolesRequest 分配角色请求。
type AssignRolesRequest struct {
	RoleIDs []int64 `json:"role_ids" binding:"required"`
}

// AssignRoles 处理 PUT /api/v1/users/:id/roles
func (h *AuthHandler) AssignRoles(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	userIDStr := c.Param("id")
	var userID int64
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID <= 0 {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	var req AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 分配角色
	if err := h.UserRepo.AssignRoles(c.Request.Context(), userID, req.RoleIDs); err != nil {
		response.Internal(c, "分配角色失败: "+err.Error())
		return
	}

	// 返回更新后的用户
	updatedUser, err := h.UserRepo.GetUserWithRoles(c.Request.Context(), userID)
	if err != nil {
		response.OK(c, gin.H{"id": userID, "message": "角色分配成功"})
		return
	}

	response.OK(c, updatedUser)
}

// UpdateUserRequest 更新用户请求。
type UpdateUserRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Status   string `json:"status"` // active / disabled
}

// UpdateUser 处理 PUT /api/v1/users/:id
func (h *AuthHandler) UpdateUser(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	userIDStr := c.Param("id")
	var userID int64
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID <= 0 {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 获取用户
	user, err := h.UserRepo.GetUserWithRoles(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "用户不存在")
		return
	}

	// 更新字段
	if req.FullName != "" {
		user.FullName = req.FullName
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Status != "" {
		if req.Status != "active" && req.Status != "disabled" {
			response.BadRequest(c, "无效的状态值，只能是 active 或 disabled")
			return
		}
		user.Status = req.Status
	}

	// 保存更新
	if err := h.UserRepo.Update(c.Request.Context(), user); err != nil {
		response.Internal(c, "更新用户失败: "+err.Error())
		return
	}

	response.OK(c, user)
}

// UpdateUserStatusRequest 更新用户状态请求。
type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// UpdateUserStatus 处理 PUT /api/v1/users/:id/status
func (h *AuthHandler) UpdateUserStatus(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	userIDStr := c.Param("id")
	var userID int64
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID <= 0 {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	var req UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 获取用户
	user, err := h.UserRepo.GetUserWithRoles(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "用户不存在")
		return
	}

	// 不允许禁用自己
	// 获取当前登录用户ID
	currentUserID, exists := c.Get("user_id")
	if exists && currentUserID.(int64) == userID && req.Status == "disabled" {
		response.BadRequest(c, "不能禁用当前登录用户")
		return
	}

	user.Status = req.Status
	if err := h.UserRepo.Update(c.Request.Context(), user); err != nil {
		response.Internal(c, "更新用户状态失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{"id": userID, "status": req.Status, "message": "用户状态更新成功"})
}

// ResetPasswordRequest 重置密码请求。
type ResetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

// ResetPassword 处理 PUT /api/v1/users/:id/password
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	if h.UserRepo == nil {
		response.Internal(c, "用户服务未初始化")
		return
	}

	userIDStr := c.Param("id")
	var userID int64
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil || userID <= 0 {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 获取用户
	user, err := h.UserRepo.GetUserWithRoles(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "用户不存在")
		return
	}

	// 加密新密码
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Internal(c, "密码加密失败: "+err.Error())
		return
	}

	user.PasswordHash = hash
	if err := h.UserRepo.Update(c.Request.Context(), user); err != nil {
		response.Internal(c, "重置密码失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{"id": userID, "message": "密码重置成功"})
}
