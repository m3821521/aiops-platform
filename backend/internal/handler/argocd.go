package handler

import (
	"strconv"

	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// ArgoCDHandler 处理 ArgoCD 请求。
type ArgoCDHandler struct {
	ArgoCD   *automation.ArgoCDClient
	Resolver automation.ArgoCDClientResolver
}

// getClient 根据请求中的 connection_id 参数获取正确的 ArgoCDClient。
func (h *ArgoCDHandler) getClient(c *gin.Context) (*automation.ArgoCDClient, error) {
	if connIDStr := c.Query("connection_id"); connIDStr != "" {
		connID, err := strconv.ParseInt(connIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		if h.Resolver == nil {
			return nil, nil
		}
		return h.Resolver.BuildArgoCDClientByID(c.Request.Context(), connID)
	}
	return h.ArgoCD, nil
}

// ListApps 处理 GET /api/v1/argocd/apps
func (h *ArgoCDHandler) ListApps(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 ArgoCD Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	apps, err := client.ListApplications(c.Request.Context())
	if err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, apps)
}

// GetApp 处理 GET /api/v1/argocd/apps/:name
func (h *ArgoCDHandler) GetApp(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 ArgoCD Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	name := c.Param("name")
	app, err := client.GetApplication(c.Request.Context(), name)
	if err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, app)
}

// SyncApp 处理 POST /api/v1/argocd/apps/:name/sync
func (h *ArgoCDHandler) SyncApp(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 ArgoCD Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	name := c.Param("name")
	if err := client.Sync(c.Request.Context(), name); err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"message": "Sync 已触发", "application": name})
}

// RefreshApp 处理 POST /api/v1/argocd/apps/:name/refresh
func (h *ArgoCDHandler) RefreshApp(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 ArgoCD Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "ArgoCD 服务未配置")
		return
	}

	name := c.Param("name")
	hard := c.Query("hard") == "true"
	if err := client.Refresh(c.Request.Context(), name, hard); err != nil {
		response.ServiceUnavailable(c, "ArgoCD 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"message": "Refresh 已触发", "application": name, "hard": hard})
}
