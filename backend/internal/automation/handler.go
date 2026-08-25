package automation

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// Handler 处理 Automation API 请求。
type Handler struct {
	Service *Service
}

// CreateActionRequest 创建 Action 的请求体。
type CreateActionRequest struct {
	IncidentID   int64                  `json:"incident_id"`
	ActionType   string                 `json:"action_type" binding:"required"`
	TargetType   string                 `json:"target_type"`
	TargetName   string                 `json:"target_name" binding:"required"`
	Cluster      string                 `json:"cluster"`
	ConnectionID *int64                 `json:"connection_id"`
	Namespace    string                 `json:"namespace"`
	Parameters   map[string]interface{} `json:"parameters"`
	Reason       string                 `json:"reason"`
	Risk         RiskLevel              `json:"risk"`
}

// Create POST /api/v1/actions
func (h *Handler) Create(c *gin.Context) {
	var req CreateActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	// action_type 白名单校验。
	// 只允许 restart_pod, scale_deployment, jenkins_build, argocd_sync。
	// 其他类型 (observe, investigate, restart, scale, rollback, config_change, network_check)
	// 不属于 Automation，应该走 Investigation / Monitoring 流程。
	if !IsSupportedActionType(req.ActionType) {
		response.BadRequest(c, fmt.Sprintf("不支持的操作类型: %s，支持的类型: %v", req.ActionType, GetSupportedActionTypes()))
		return
	}

	userID := getUserID(c)
	action := &Action{
		IncidentID:   req.IncidentID,
		ActionType:   req.ActionType,
		TargetType:   req.TargetType,
		TargetName:   req.TargetName,
		Cluster:      req.Cluster,
		ConnectionID: req.ConnectionID,
		Namespace:    req.Namespace,
		Reason:       req.Reason,
		Risk:         req.Risk,
	}
	action.SetParameters(req.Parameters)

	result, err := h.Service.CreateAction(c.Request.Context(), action, userID)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, result)
}

// List GET /api/v1/actions
func (h *Handler) List(c *gin.Context) {
	filter := ListFilter{}
	if s := c.Query("status"); s != "" {
		filter.Status = ActionStatus(s)
	}
	if r := c.Query("risk"); r != "" {
		filter.Risk = RiskLevel(r)
	}
	if t := c.Query("action_type"); t != "" {
		filter.ActionType = ActionType(t)
	}
	if i := c.Query("incident_id"); i != "" {
		filter.IncidentID, _ = strconv.ParseInt(i, 10, 64)
	}
	if cl := c.Query("cluster"); cl != "" {
		filter.Cluster = cl
	}

	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if ps := c.Query("page_size"); ps != "" {
		pageSize, _ = strconv.Atoi(ps)
	}

	actions, total, err := h.Service.ListActions(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}

	var sourceUpdatedAt time.Time
	hasUpdated := false
	for i := range actions {
		if !hasUpdated || actions[i].UpdatedAt.After(sourceUpdatedAt) {
			sourceUpdatedAt = actions[i].UpdatedAt
			hasUpdated = true
		}
	}

	fetchedAt := time.Now()
	prov := &response.Provenance{
		Source:             "mysql",
		SourceType:         "mysql",
		FetchedAt:          &fetchedAt,
		TimestampAvailable: false,
		TimestampSemantics: "latest_record_updated_at",
	}
	if hasUpdated {
		su := sourceUpdatedAt
		prov.SourceUpdatedAt = &su
	}
	response.OKWithProvenance(c, gin.H{"items": actions, "total": total, "page": page, "page_size": pageSize}, prov)
}

// Get GET /api/v1/actions/:id
func (h *Handler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	action, err := h.Service.GetAction(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Action 不存在")
		return
	}
	response.OK(c, action)
}

// Approve POST /api/v1/actions/:id/approve
func (h *Handler) Approve(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	userID := getUserID(c)
	result, err := h.Service.Approve(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// RejectRequest 拒绝请求体。
type RejectRequest struct {
	Reason string `json:"reason"`
}

// Reject POST /api/v1/actions/:id/reject
func (h *Handler) Reject(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	userID := getUserID(c)
	var req RejectRequest
	c.ShouldBindJSON(&req)
	result, err := h.Service.Reject(c.Request.Context(), id, userID, req.Reason)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// DryRun POST /api/v1/actions/:id/dry-run
func (h *Handler) DryRun(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	result, err := h.Service.DryRun(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Execute POST /api/v1/actions/:id/execute
func (h *Handler) Execute(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	userID := getUserID(c)
	result, err := h.Service.Execute(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Cancel POST /api/v1/actions/:id/cancel
func (h *Handler) Cancel(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	userID := getUserID(c)
	result, err := h.Service.Cancel(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Executions GET /api/v1/actions/:id/executions
func (h *Handler) Executions(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	execs, err := h.Service.ListExecutions(c.Request.Context(), id)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, execs)
}

// PendingApproval GET /api/v1/actions/pending-approval
func (h *Handler) PendingApproval(c *gin.Context) {
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if ps := c.Query("page_size"); ps != "" {
		pageSize, _ = strconv.Atoi(ps)
	}

	actions, total, err := h.Service.ListActions(c.Request.Context(), ListFilter{Status: StatusPendingApproval}, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, gin.H{"items": actions, "total": total, "page": page, "page_size": pageSize})
}

// CreateFromIncident POST /api/v1/incidents/:id/actions
func (h *Handler) CreateFromIncident(c *gin.Context) {
	incidentID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req CreateActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}

	// action_type 白名单校验。
	if !IsSupportedActionType(req.ActionType) {
		response.BadRequest(c, fmt.Sprintf("不支持的操作类型: %s，支持的类型: %v", req.ActionType, GetSupportedActionTypes()))
		return
	}

	userID := getUserID(c)
	action := &Action{
		IncidentID:   incidentID,
		ActionType:   req.ActionType,
		TargetType:   req.TargetType,
		TargetName:   req.TargetName,
		Cluster:      req.Cluster,
		ConnectionID: req.ConnectionID,
		Namespace:    req.Namespace,
		Reason:       req.Reason,
		Risk:         req.Risk,
	}
	action.SetParameters(req.Parameters)

	result, err := h.Service.CreateAction(c.Request.Context(), action, userID)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Audit GET /api/v1/automation/audit
func (h *Handler) Audit(c *gin.Context) {
	var actionID, incidentID, userID int64
	if a := c.Query("action_id"); a != "" {
		actionID, _ = strconv.ParseInt(a, 10, 64)
	}
	if i := c.Query("incident_id"); i != "" {
		incidentID, _ = strconv.ParseInt(i, 10, 64)
	}
	if u := c.Query("user_id"); u != "" {
		userID, _ = strconv.ParseInt(u, 10, 64)
	}
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if ps := c.Query("page_size"); ps != "" {
		pageSize, _ = strconv.Atoi(ps)
	}

	audits, total, err := h.Service.ListAudit(c.Request.Context(), actionID, incidentID, userID, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, gin.H{"items": audits, "total": total, "page": page, "page_size": pageSize})
}

// getUserID 从 JWT context 获取用户 ID。
func getUserID(c *gin.Context) int64 {
	user := auth.CurrentUser(c)
	if user != nil {
		return user.ID
	}
	return 0
}

// getCurrentUser 获取当前用户对象。
func getCurrentUser(c *gin.Context) *auth.User {
	return auth.CurrentUser(c)
}
