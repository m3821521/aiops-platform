package workflow

import (
	"strconv"

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
	response.OK(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
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
