package handler

import (
	"strconv"

	"github.com/aiops/aiops-platform/internal/audit"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AuditHandler 处理审计日志请求。
type AuditHandler struct {
	Repo *audit.Repository
}

// List 处理 GET /api/v1/audit-logs
func (h *AuditHandler) List(c *gin.Context) {
	if h.Repo == nil {
		response.Internal(c, "审计服务未初始化")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := audit.ListFilter{
		Username: c.Query("username"),
		Action:   c.Query("action"),
		Resource: c.Query("resource"),
		Result:   c.Query("result"),
	}

	logs, total, err := h.Repo.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.Internal(c, "查询审计日志失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"items":     logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
