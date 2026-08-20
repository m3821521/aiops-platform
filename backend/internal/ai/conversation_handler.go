package ai

import (
	"context"
	"strconv"
	"time"

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

// ListConversations 处理 GET /api/v1/ai/conversations。
func (h *ConversationHandler) ListConversations(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)
	if uid == 0 {
		uid = 1 // 默认 admin 用户
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
		c.JSON(500, gin.H{"code": "INTERNAL_ERROR", "message": "查询对话列表失败"})
		return
	}

	c.JSON(200, gin.H{
		"code": "OK",
		"data": gin.H{
			"items": convs,
			"total": total,
			"page":  page,
			"page_size": pageSize,
		},
	})
}

// GetConversation 处理 GET /api/v1/ai/conversations/:id。
func (h *ConversationHandler) GetConversation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"code": "BAD_REQUEST", "message": "无效的对话 ID"})
		return
	}

	conv, err := h.Repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(404, gin.H{"code": "NOT_FOUND", "message": "对话不存在"})
		return
	}

	// 验证所有权。
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)
	if uid != 0 && conv.UserID != uid {
		c.JSON(403, gin.H{"code": "FORBIDDEN", "message": "无权访问此对话"})
		return
	}

	msgs, err := h.Repo.GetMessages(c.Request.Context(), id)
	if err != nil {
		c.JSON(500, gin.H{"code": "INTERNAL_ERROR", "message": "查询消息失败"})
		return
	}

	c.JSON(200, gin.H{
		"code": "OK",
		"data": gin.H{
			"conversation": conv,
			"messages":     msgs,
		},
	})
}

// DeleteConversation 处理 DELETE /api/v1/ai/conversations/:id。
func (h *ConversationHandler) DeleteConversation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"code": "BAD_REQUEST", "message": "无效的对话 ID"})
		return
	}

	conv, err := h.Repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(404, gin.H{"code": "NOT_FOUND", "message": "对话不存在"})
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)
	if uid != 0 && conv.UserID != uid {
		c.JSON(403, gin.H{"code": "FORBIDDEN", "message": "无权删除此对话"})
		return
	}

	if err := h.Repo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(500, gin.H{"code": "INTERNAL_ERROR", "message": "删除对话失败"})
		return
	}

	c.JSON(200, gin.H{"code": "OK", "message": "对话已删除"})
}

// CreateConversation 创建新对话（内部使用）。
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
