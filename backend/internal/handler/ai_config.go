package handler

import (
	"net/http"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AIConfigHandler 处理 AI 配置的 HTTP 请求。
type AIConfigHandler struct {
	Repo       *ai.AIConfigRepository
	Provider   *ai.OpenAIProvider // 用于运行时更新 API Key
	OnUpdate   func()             // 配置更新后的回调（通知 AIHandler 重新加载）
}

// AIConfigResponse 是返回给前端的配置状态（不包含明文 API Key）。
type AIConfigResponse struct {
	Provider     string `json:"provider"`
	BaseURL      string `json:"base_url"`
	Model        string `json:"model"`
	Enabled      bool   `json:"enabled"`
	APIKeySet    bool   `json:"api_key_set"` // 是否已配置 API Key（不返回明文）
	APIKeyMasked string `json:"api_key_masked,omitempty"` // 脱敏后的 API Key，如 sk-****abcd
}

// GetConfig 处理 GET /api/v1/ai/config
// 返回当前 AI 配置状态，不返回明文 API Key。
func (h *AIConfigHandler) GetConfig(c *gin.Context) {
	if h.Repo == nil {
		response.NotFound(c, "AI 配置存储未初始化")
		return
	}

	cfg, err := h.Repo.Get(c.Request.Context())
	if err != nil {
		response.Internal(c, "获取 AI 配置失败")
		return
	}

	if cfg == nil {
		// 未配置，返回默认状态
		c.JSON(http.StatusOK, response.Body{
			Code:    0,
			Message: "success",
			Data: AIConfigResponse{
				Provider:  "openai",
				BaseURL:   "https://api.openai.com/v1",
				Model:     "gpt-4o-mini",
				Enabled:   false,
				APIKeySet: false,
			},
		})
		return
	}

	apiKey, _ := h.Repo.GetAPIKey(c.Request.Context())
	resp := AIConfigResponse{
		Provider:  cfg.Provider,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		Enabled:   cfg.Enabled,
		APIKeySet: apiKey != "",
	}
	if apiKey != "" && len(apiKey) > 8 {
		resp.APIKeyMasked = apiKey[:6] + "****" + apiKey[len(apiKey)-4:]
	}

	c.JSON(http.StatusOK, response.Body{
		Code:    0,
		Message: "success",
		Data:    resp,
	})
}

// UpdateConfigRequest 是更新 AI 配置的请求体。
type UpdateConfigRequest struct {
	Provider string `json:"provider"` // 不限制取值，支持各种 OpenAI 兼容 API（DeepSeek、通义千问、智谱等）
	BaseURL  string `json:"base_url" binding:"omitempty,url"`
	APIKey   string `json:"api_key" binding:"omitempty,min=10"`
	Model    string `json:"model" binding:"omitempty,min=1"`
	Enabled  *bool  `json:"enabled"`
}

// UpdateConfig 处理 POST /api/v1/ai/config
// 更新 AI 配置，API Key 会被加密存储。
func (h *AIConfigHandler) UpdateConfig(c *gin.Context) {
	if h.Repo == nil {
		response.NotFound(c, "AI 配置存储未初始化")
		return
	}

	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 获取当前配置作为默认值
	current, err := h.Repo.Get(c.Request.Context())
	if err != nil {
		response.Internal(c, "获取当前 AI 配置失败")
		return
	}

	provider := "openai"
	baseURL := "https://api.openai.com/v1"
	model := "gpt-4o-mini"
	enabled := true
	if current != nil {
		provider = current.Provider
		baseURL = current.BaseURL
		model = current.Model
		enabled = current.Enabled
	}

	if req.Provider != "" {
		provider = req.Provider
	}
	if req.BaseURL != "" {
		baseURL = req.BaseURL
	}
	if req.Model != "" {
		model = req.Model
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 获取用户 ID
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	// 保存配置
	cfg, err := h.Repo.Save(c.Request.Context(), provider, baseURL, req.APIKey, model, enabled, uid)
	if err != nil {
		response.Internal(c, "保存 AI 配置失败")
		return
	}

	// 运行时更新 Provider 的 API Key
	if h.Provider != nil && req.APIKey != "" {
		h.Provider.SetAPIKey(req.APIKey)
		h.Provider.SetModel(model)
	}

	// 触发回调通知 AIHandler
	if h.OnUpdate != nil {
		h.OnUpdate()
	}

	apiKey, _ := h.Repo.GetAPIKey(c.Request.Context())
	resp := AIConfigResponse{
		Provider:  cfg.Provider,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		Enabled:   cfg.Enabled,
		APIKeySet: apiKey != "",
	}
	if apiKey != "" && len(apiKey) > 8 {
		resp.APIKeyMasked = apiKey[:6] + "****" + apiKey[len(apiKey)-4:]
	}

	c.JSON(http.StatusOK, response.Body{
		Code:    0,
		Message: "AI 配置已更新",
		Data:    resp,
	})
}
