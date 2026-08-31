package ai

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// ConversationHandler 处理对话相关 API。
type ConversationHandler struct {
	Repo *ConversationRepository
}

// NewConversationHandler 创建对话 Handler。
func NewConversationHandler(repo *ConversationRepository) *ConversationHandler {
	return &ConversationHandler{Repo: repo}
}

// getAuthenticatedUserID 从 gin context 中提取当前认证用户 ID。
// 如果未认证（用户不存在或 ID 为 0），返回 (0, false)。
func getAuthenticatedUserID(c *gin.Context) (int64, bool) {
	user := auth.CurrentUser(c)
	if user == nil || user.ID <= 0 {
		return 0, false
	}
	return user.ID, true
}

// ListConversations 处理 GET /api/v1/ai/conversations。
func (h *ConversationHandler) ListConversations(c *gin.Context) {
	uid, ok := getAuthenticatedUserID(c)
	if !ok {
		response.Unauthorized(c, "未登录或登录已过期")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	convs, total, err := h.Repo.ListByUser(c.Request.Context(), uid, page, pageSize)
	if err != nil {
		slog.Error("ai: list conversations failed", "user_id", uid, "err", err)
		response.Internal(c, "查询对话列表失败")
		return
	}

	response.OK(c, gin.H{
		"items":     convs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetConversation 处理 GET /api/v1/ai/conversations/:id。
func (h *ConversationHandler) GetConversation(c *gin.Context) {
	uid, ok := getAuthenticatedUserID(c)
	if !ok {
		response.Unauthorized(c, "未登录或登录已过期")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的对话 ID")
		return
	}

	// 使用 GetByIDAndUser 防止 IDOR：不存在或不属于当前用户都返回 404。
	conv, err := h.Repo.GetByIDAndUser(c.Request.Context(), id, uid)
	if err != nil {
		response.NotFound(c, "对话不存在")
		return
	}

	msgs, err := h.Repo.GetMessages(c.Request.Context(), id)
	if err != nil {
		slog.Error("ai: get conversation messages failed", "conversation_id", id, "err", err)
		response.Internal(c, "查询对话消息失败")
		return
	}

	response.OK(c, gin.H{
		"conversation": conv,
		"messages":     msgs,
	})
}

// DeleteConversation 处理 DELETE /api/v1/ai/conversations/:id。
func (h *ConversationHandler) DeleteConversation(c *gin.Context) {
	uid, ok := getAuthenticatedUserID(c)
	if !ok {
		response.Unauthorized(c, "未登录或登录已过期")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的对话 ID")
		return
	}

	// 先验证所有权，防止 IDOR 删除他人对话。
	if _, err := h.Repo.GetByIDAndUser(c.Request.Context(), id, uid); err != nil {
		response.NotFound(c, "对话不存在")
		return
	}

	if err := h.Repo.Delete(c.Request.Context(), id); err != nil {
		slog.Error("ai: delete conversation failed", "conversation_id", id, "user_id", uid, "err", err)
		response.Internal(c, "删除对话失败")
		return
	}

	response.OK(c, gin.H{"message": "对话已删除"})
}

// CreateConversation 创建新对话（内部使用，由 AIHandler 在 ask 时调用）。
func (h *ConversationHandler) CreateConversation(ctx context.Context, userID int64, title string, incidentID *int64) (*Conversation, error) {
	conv := &Conversation{
		UserID:     userID,
		Title:      title,
		IncidentID: incidentID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := h.Repo.Create(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}
