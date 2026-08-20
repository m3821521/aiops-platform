package agent

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/aiops/aiops-platform/pkg/response"
)

// Handler 是多 Agent 编排的 HTTP Handler。
type Handler struct {
	Orchestrator *Orchestrator
	Registry     *Registry
}

// NewHandler 创建新的多 Agent Handler。
func NewHandler(orchestrator *Orchestrator, registry *Registry) *Handler {
	return &Handler{
		Orchestrator: orchestrator,
		Registry:     registry,
	}
}

// OrchestrateRequest 是编排请求。
type OrchestrateRequest struct {
	Title       string                 `json:"title" binding:"required"`
	Description string                 `json:"description"`
	IncidentID  int64                  `json:"incident_id,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	AgentTypes  []string               `json:"agent_types,omitempty"`
	AutoApprove bool                   `json:"auto_approve,omitempty"`
}

// Orchestrate 处理 POST /api/v1/agent/orchestrate
func (h *Handler) Orchestrate(c *gin.Context) {
	var req OrchestrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 转换 Agent 类型
	var agentTypes []AgentType
	for _, t := range req.AgentTypes {
		agentTypes = append(agentTypes, AgentType(t))
	}

	// 获取当前用户
	var userID int64
	var username string
	if uid, exists := c.Get("user_id"); exists && uid != nil {
		if id, ok := uid.(int64); ok {
			userID = id
		}
	}
	if uname, exists := c.Get("username"); exists && uname != nil {
		if name, ok := uname.(string); ok {
			username = name
		}
	}

	orchestrationReq := &OrchestrationRequest{
		Title:        req.Title,
		Description:  req.Description,
		IncidentID:   req.IncidentID,
		UserID:       userID,
		Username:     username,
		Parameters:   req.Parameters,
		AgentTypes:   agentTypes,
		AutoApprove:  req.AutoApprove,
	}

	result, err := h.Orchestrator.Orchestrate(c.Request.Context(), orchestrationReq)
	if err != nil {
		response.Internal(c, "多 Agent 编排失败: "+err.Error())
		return
	}

	response.OK(c, result)
}

// GetResult 处理 GET /api/v1/agent/results/:id
func (h *Handler) GetResult(c *gin.Context) {
	taskID := c.Param("id")

	result, exists := h.Orchestrator.GetResult(taskID)
	if !exists {
		response.NotFound(c, "编排任务不存在")
		return
	}

	response.OK(c, result)
}

// ListResults 处理 GET /api/v1/agent/results
func (h *Handler) ListResults(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results := h.Orchestrator.ListResults()

	// 简单分页
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(results) {
		start = len(results)
	}
	if end > len(results) {
		end = len(results)
	}

	pagedResults := results[start:end]

	response.OK(c, gin.H{
		"items":     pagedResults,
		"total":     len(results),
		"page":      page,
		"page_size": pageSize,
	})
}

// GetEvents 处理 GET /api/v1/agent/results/:id/events
func (h *Handler) GetEvents(c *gin.Context) {
	taskID := c.Param("id")

	events := h.Orchestrator.GetEvents(taskID)

	response.OK(c, gin.H{
		"items": events,
		"total": len(events),
	})
}

// ListAgents 处理 GET /api/v1/agents
func (h *Handler) ListAgents(c *gin.Context) {
	agents := h.Registry.GetAll()

	type AgentInfo struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Description  string   `json:"description"`
		Capabilities []string `json:"capabilities"`
	}

	var agentInfos []AgentInfo
	for _, agent := range agents {
		agentInfos = append(agentInfos, AgentInfo{
			Name:         agent.Name(),
			Type:         string(agent.Type()),
			Description:  agent.Description(),
			Capabilities: agent.Capabilities(),
		})
	}

	response.OK(c, agentInfos)
}

// ApproveRequest 是批准请求。
type ApproveRequest struct {
	Approver string `json:"approver"`
	Reason   string `json:"reason,omitempty"`
}

// Approve 处理 POST /api/v1/agent/results/:id/approve
func (h *Handler) Approve(c *gin.Context) {
	taskID := c.Param("id")

	var req ApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	approver := req.Approver
	if approver == "" {
		username, _ := c.Get("username")
		approver = username.(string)
	}

	result, err := h.Orchestrator.ApproveTask(taskID, approver)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, result)
}

// Reject 处理 POST /api/v1/agent/results/:id/reject
func (h *Handler) Reject(c *gin.Context) {
	taskID := c.Param("id")

	var req ApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	rejector := req.Approver
	if rejector == "" {
		username, _ := c.Get("username")
		rejector = username.(string)
	}

	result, err := h.Orchestrator.RejectTask(taskID, rejector, req.Reason)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, result)
}

// RegisterRoutes 注册多 Agent 编排路由。
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	agentGroup := r.Group("/agent")
	{
		agentGroup.POST("/orchestrate", h.Orchestrate)
		agentGroup.GET("/results", h.ListResults)
		agentGroup.GET("/results/:id", h.GetResult)
		agentGroup.GET("/results/:id/events", h.GetEvents)
		agentGroup.POST("/results/:id/approve", h.Approve)
		agentGroup.POST("/results/:id/reject", h.Reject)
	}

	agentsGroup := r.Group("/agents")
	{
		agentsGroup.GET("", h.ListAgents)
	}
}

// Ensure http import is used
var _ = http.StatusOK
