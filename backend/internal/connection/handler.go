package connection

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/aiops/aiops-platform/pkg/response"
)

// Handler 是 Connection 和 Credential 的 HTTP Handler。
type Handler struct {
	connectionService *ConnectionService
	credentialService *CredentialService
	connectionManager *ConnectionManager
	providerRegistry  *ProviderRegistry
	healthChecker     *HealthChecker
}

// NewHandler 创建 Handler。
func NewHandler(
	connectionService *ConnectionService,
	credentialService *CredentialService,
	connectionManager *ConnectionManager,
	providerRegistry *ProviderRegistry,
) *Handler {
	return &Handler{
		connectionService: connectionService,
		credentialService: credentialService,
		connectionManager: connectionManager,
		providerRegistry:  providerRegistry,
	}
}

// SetHealthChecker 注入健康检查器（可选，用于 Batch Health Check API）。
func (h *Handler) SetHealthChecker(checker *HealthChecker) {
	h.healthChecker = checker
}

// ============================================================================
// Connection CRUD
// ============================================================================

// ListConnections 处理 GET /api/v1/connections
func (h *Handler) ListConnections(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := ConnectionFilter{
		Type:        ConnectionType(c.Query("type")),
		Environment: Environment(c.Query("environment")),
		Name:        c.Query("name"),
	}

	if status := c.Query("status"); status != "" {
		filter.Status = ConnectionStatus(status)
	}
	if enabled := c.Query("enabled"); enabled != "" {
		e := enabled == "true"
		filter.Enabled = &e
	}

	views, total, err := h.connectionManager.ListAll(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, "查询 connection 列表失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"items":     views,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetConnection 处理 GET /api/v1/connections/:id
func (h *Handler) GetConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 connection ID")
		return
	}

	view, err := h.connectionService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, view)
}

// CreateConnection 处理 POST /api/v1/connections
func (h *Handler) CreateConnection(c *gin.Context) {
	var req CreateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	userID := getUserID(c)

	view, err := h.connectionService.Create(c.Request.Context(), req, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Created(c, view)
}

// UpdateConnection 处理 PUT /api/v1/connections/:id
func (h *Handler) UpdateConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 connection ID")
		return
	}

	var req UpdateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	userID := getUserID(c)

	view, err := h.connectionService.Update(c.Request.Context(), id, req, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, view)
}

// DeleteConnection 处理 DELETE /api/v1/connections/:id
func (h *Handler) DeleteConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 connection ID")
		return
	}

	if err := h.connectionService.Delete(c.Request.Context(), id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "connection 已删除"})
}

// EnableConnection 处理 POST /api/v1/connections/:id/enable
func (h *Handler) EnableConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 connection ID")
		return
	}

	userID := getUserID(c)

	view, err := h.connectionService.Enable(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, view)
}

// DisableConnection 处理 POST /api/v1/connections/:id/disable
func (h *Handler) DisableConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 connection ID")
		return
	}

	userID := getUserID(c)

	view, err := h.connectionService.Disable(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, view)
}

// TestConnection 处理 POST /api/v1/connections/:id/test
func (h *Handler) TestConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 connection ID")
		return
	}

	// 获取 Connection
	conn, err := h.connectionService.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Internal(c, "查询 connection 失败: "+err.Error())
		return
	}
	if conn == nil {
		response.NotFound(c, "connection 不存在")
		return
	}

	// 执行连接测试
	result, err := h.providerRegistry.TestConnection(c.Request.Context(), conn)
	if err != nil {
		response.Internal(c, "连接测试失败: "+err.Error())
		return
	}

	// 更新 Connection 状态
	if err := h.connectionService.UpdateStatus(c.Request.Context(), id, result.Status, result.ErrorMessage); err != nil {
		// 状态更新失败不影响测试结果返回，但记录日志
		slog.Warn("更新连接状态失败", "connection_id", id, "error", err)
	}

	response.OK(c, result)
}

// ListConnectionTypes 处理 GET /api/v1/connections/types
// 返回支持的 Connection 类型和已注册的 Provider。
func (h *Handler) ListConnectionTypes(c *gin.Context) {
	types := []ConnectionType{
		TypeKubernetes,
		TypePrometheus,
		TypeGrafana,
		TypeElasticsearch,
		TypeMySQL,
		TypeRedis,
		TypeJenkins,
		TypeArgoCD,
		TypeDocker,
	}

	registeredProviders := h.providerRegistry.List()
	registeredMap := make(map[ConnectionType]bool)
	for _, t := range registeredProviders {
		registeredMap[t] = true
	}

	type TypeInfo struct {
		Type       ConnectionType `json:"type"`
		Registered bool           `json:"registered"`
	}

	result := make([]TypeInfo, len(types))
	for i, t := range types {
		result[i] = TypeInfo{
			Type:       t,
			Registered: registeredMap[t],
		}
	}

	response.OK(c, result)
}

// ============================================================================
// Credential CRUD
// ============================================================================

// ListCredentials 处理 GET /api/v1/credentials
func (h *Handler) ListCredentials(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	views, total, err := h.credentialService.List(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Internal(c, "查询 credential 列表失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"items":     views,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetCredential 处理 GET /api/v1/credentials/:id
func (h *Handler) GetCredential(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 credential ID")
		return
	}

	view, err := h.credentialService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, view)
}

// CreateCredential 处理 POST /api/v1/credentials
func (h *Handler) CreateCredential(c *gin.Context) {
	var req CreateCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	userID := getUserID(c)

	view, err := h.credentialService.Create(c.Request.Context(), req, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Created(c, view)
}

// UpdateCredential 处理 PUT /api/v1/credentials/:id
func (h *Handler) UpdateCredential(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 credential ID")
		return
	}

	var req UpdateCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	userID := getUserID(c)

	view, err := h.credentialService.Update(c.Request.Context(), id, req, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, view)
}

// DeleteCredential 处理 DELETE /api/v1/credentials/:id
func (h *Handler) DeleteCredential(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 credential ID")
		return
	}

	if err := h.credentialService.Delete(c.Request.Context(), id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "credential 已删除"})
}

// ============================================================================
// Health Check
// ============================================================================

// BatchHealthCheck 处理 POST /api/v1/connections/health-check
// 立即对所有 enabled 的 Connection 执行真实健康检查。
// 返回检查结果，不包含任何 credential / password / token 信息。
func (h *Handler) BatchHealthCheck(c *gin.Context) {
	if h.healthChecker == nil {
		response.Internal(c, "health checker 未初始化")
		return
	}

	slog.Info("batch health check triggered", "user_id", getUserID(c))

	results := h.healthChecker.CheckAll(c.Request.Context())
	if results == nil {
		results = []HealthCheckResult{}
	}

	available := 0
	unavailable := 0
	for _, r := range results {
		if r.Status == StatusAvailable {
			available++
		} else if r.Status == StatusUnavailable {
			unavailable++
		}
	}

	response.OK(c, gin.H{
		"items":       results,
		"total":       len(results),
		"available":   available,
		"unavailable": unavailable,
		"checked_at":  time.Now(),
	})
}

// getUserID 从 Gin Context 获取当前用户 ID。
func getUserID(c *gin.Context) int64 {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(int64); ok {
			return id
		}
	}
	return 0
}

// Ensure context import is used
var _ = context.Background

// Ensure http import is used
var _ = http.StatusOK
