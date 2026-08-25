package workflow

import (
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	Service *Service
}

type CreateWorkflowRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	IncidentID  int64                  `json:"incident_id"`
	Risk        string                 `json:"risk"`
	Steps       []WorkflowStep         `json:"steps" binding:"required"`
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}
	user := auth.CurrentUser(c)
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}

	wf := &Workflow{
		Name:        req.Name,
		Description: req.Description,
		IncidentID:  req.IncidentID,
		Risk:        req.Risk,
		Steps:       req.Steps,
	}
	result, err := h.Service.CreateWorkflow(c.Request.Context(), wf, userID)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if ps := c.Query("page_size"); ps != "" {
		pageSize, _ = strconv.Atoi(ps)
	}
	filter := ListFilter{}
	if s := c.Query("status"); s != "" {
		filter.Status = WorkflowStatus(s)
	}
	if incID := c.Query("incident_id"); incID != "" {
		filter.IncidentID, _ = strconv.ParseInt(incID, 10, 64)
	}

	items, total, err := h.Service.repo.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}

	var sourceUpdatedAt time.Time
	hasUpdated := false
	for i := range items {
		if !hasUpdated || items[i].UpdatedAt.After(sourceUpdatedAt) {
			sourceUpdatedAt = items[i].UpdatedAt
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
	response.OKWithProvenance(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}, prov)
}

func (h *Handler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	wf, err := h.Service.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Workflow 不存在")
		return
	}
	response.OK(c, wf)
}

func (h *Handler) Submit(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user := auth.CurrentUser(c)
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}
	result, err := h.Service.Submit(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

func (h *Handler) Approve(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user := auth.CurrentUser(c)
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}
	result, err := h.Service.Approve(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

func (h *Handler) Execute(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user := auth.CurrentUser(c)
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}
	result, err := h.Service.Execute(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

func (h *Handler) Cancel(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user := auth.CurrentUser(c)
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}
	result, err := h.Service.Cancel(c.Request.Context(), id, userID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// DryRun 模拟执行工作流，不真正修改任何资源。
func (h *Handler) DryRun(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	result, err := h.Service.DryRun(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// ListExecutions 获取工作流的执行记录列表。
func (h *Handler) ListExecutions(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if ps := c.Query("page_size"); ps != "" {
		pageSize, _ = strconv.Atoi(ps)
	}
	items, total, err := h.Service.GetExecutions(c.Request.Context(), id, page, pageSize)
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

// GetExecution 获取单个工作流执行记录。
func (h *Handler) GetExecution(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	exec, err := h.Service.GetExecution(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "工作流执行记录不存在")
		return
	}
	response.OK(c, exec)
}
