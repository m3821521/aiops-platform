package handler

import (
	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// ArgoCDHandler 处理 ArgoCD 请求。
type ArgoCDHandler struct {
	ArgoCD *automation.ArgoCDClient
}

// ListApps 处理 GET /api/v1/argocd/apps
func (h *ArgoCDHandler) ListApps(c *gin.Context) {
	if h.ArgoCD == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	apps, err := h.ArgoCD.ListApplications(c.Request.Context())
	if err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, apps)
}

// GetApp 处理 GET /api/v1/argocd/apps/:name
func (h *ArgoCDHandler) GetApp(c *gin.Context) {
	if h.ArgoCD == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	name := c.Param("name")
	app, err := h.ArgoCD.GetApplication(c.Request.Context(), name)
	if err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, app)
}

// SyncApp 处理 POST /api/v1/argocd/apps/:name/sync
func (h *ArgoCDHandler) SyncApp(c *gin.Context) {
	if h.ArgoCD == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	name := c.Param("name")
	if err := h.ArgoCD.Sync(c.Request.Context(), name); err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"message": "Sync 已触发", "application": name})
}

// RefreshApp 处理 POST /api/v1/argocd/apps/:name/refresh
func (h *ArgoCDHandler) RefreshApp(c *gin.Context) {
	if h.ArgoCD == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	name := c.Param("name")
	hard := c.Query("hard") == "true"
	if err := h.ArgoCD.Refresh(c.Request.Context(), name, hard); err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"message": "Refresh 已触发", "application": name, "hard": hard})
}
