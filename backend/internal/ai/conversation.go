package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Conversation 表示一次 AI 对话会话。
type Conversation struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64          `gorm:"index;not null" json:"user_id"`
	Title       string         `gorm:"size:255" json:"title"`
	IncidentID  *int64         `gorm:"index" json:"incident_id,omitempty"`
	MessageCount int           `gorm:"default:0" json:"message_count"`
	CreatedAt   time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// ConversationMessage 表示对话中的一条消息。
type ConversationMessage struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID int64     `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:20;not null" json:"role"` // user / assistant
	Content        string    `gorm:"type:text" json:"content"`
	Summary        string    `gorm:"type:text" json:"summary,omitempty"`
	RootCause      string    `gorm:"type:text" json:"root_cause,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	EvidenceJSON   string    `gorm:"type:text" json:"-"`
	RecommendJSON  string    `gorm:"type:text" json:"-"`
	ToolCallsJSON  string    `gorm:"type:text" json:"-"`
	DurationMs     int64     `json:"duration_ms,omitempty"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}

// ConversationRepository 提供对话持久化。
type ConversationRepository struct {
	db *gorm.DB
}

// NewConversationRepository 创建对话仓库。
func NewConversationRepository(db *gorm.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

// Create 创建新对话。
func (r *ConversationRepository) Create(ctx context.Context, conv *Conversation) error {
	return r.db.WithContext(ctx).Create(conv).Error
}

// GetByID 按 ID 获取对话。
func (r *ConversationRepository) GetByID(ctx context.Context, id int64) (*Conversation, error) {
	var conv Conversation
	if err := r.db.WithContext(ctx).First(&conv, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, err
	}
	return &conv, nil
}

// GetByIDAndUser 按 ID 和用户 ID 获取对话，用于防止 IDOR 越权访问。
// 不存在或不属于该用户时返回 "conversation not found"（不区分两种情况，防止信息泄露）。
func (r *ConversationRepository) GetByIDAndUser(ctx context.Context, id, userID int64) (*Conversation, error) {
	var conv Conversation
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&conv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, err
	}
	return &conv, nil
}

// ListByUser 获取用户的对话列表。
func (r *ConversationRepository) ListByUser(ctx context.Context, userID int64, page, pageSize int) ([]Conversation, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&Conversation{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var convs []Conversation
	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("updated_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&convs).Error; err != nil {
		return nil, 0, err
	}
	return convs, total, nil
}

// Delete 删除对话（软删除）。
func (r *ConversationRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Conversation{}, id).Error
}

// UpdateTitle 更新对话标题。
func (r *ConversationRepository) UpdateTitle(ctx context.Context, id int64, title string) error {
	return r.db.WithContext(ctx).Model(&Conversation{}).Where("id = ?", id).
		Updates(map[string]interface{}{"title": title, "updated_at": time.Now()}).Error
}

// AddMessage 添加消息到对话。
func (r *ConversationRepository) AddMessage(ctx context.Context, msg *ConversationMessage) error {
	if err := r.db.WithContext(ctx).Create(msg).Error; err != nil {
		return err
	}
	// 更新对话的消息计数和更新时间。
	return r.db.WithContext(ctx).Model(&Conversation{}).Where("id = ?", msg.ConversationID).
		Updates(map[string]interface{}{
			"message_count": gorm.Expr("message_count + 1"),
			"updated_at":    time.Now(),
		}).Error
}

// GetMessages 获取对话的所有消息。
func (r *ConversationRepository) GetMessages(ctx context.Context, conversationID int64) ([]ConversationMessage, error) {
	var msgs []ConversationMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("created_at ASC").Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// GetRecentMessages 获取最近 N 条消息（用于 context window）。
func (r *ConversationRepository) GetRecentMessages(ctx context.Context, conversationID int64, limit int) ([]ConversationMessage, error) {
	var msgs []ConversationMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("created_at DESC").Limit(limit).Find(&msgs).Error; err != nil {
		return nil, err
	}
	// 反转顺序，使最早的消息在前。
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// MarshalEvidence 将证据序列化为 JSON。
func MarshalEvidence(evidence interface{}) string {
	b, _ := json.Marshal(evidence)
	return string(b)
}

// UnmarshalEvidence 反序列化证据。
func UnmarshalEvidence(data string) []map[string]interface{} {
	if data == "" {
		return nil
	}
	var result []map[string]interface{}
	_ = json.Unmarshal([]byte(data), &result)
	return result
}
