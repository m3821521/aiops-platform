package handler

import (
	"strconv"

	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// AutomationHandler 处理自动化运维请求。
type AutomationHandler struct {
	Engine *automation.Engine
}

// GetPodLogs 处理 GET /api/v1/automation/pods/:pod/logs
func (h *AutomationHandler) GetPodLogs(c *gin.Context) {
	if h.Engine == nil {
		response.Internal(c, "自动化引擎未初始化")
		return
	}

	pod := c.Param("pod")
	cluster := c.DefaultQuery("cluster", "")
	namespace := c.DefaultQuery("namespace", "default")
	container := c.Query("container")
	tail := int64(100)
	if t := c.Query("tail"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil && v > 0 {
			tail = v
		}
	}

	logs, err := h.Engine.GetPodLogs(c.Request.Context(), cluster, namespace, pod, container, tail)
	if err != nil {
		response.Internal(c, "获取 Pod 日志失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{"logs": logs})
}

// GetPodEvents 处理 GET /api/v1/automation/pods/:pod/events
func (h *AutomationHandler) GetPodEvents(c *gin.Context) {
	if h.Engine == nil {
		response.Internal(c, "自动化引擎未初始化")
		return
	}

	pod := c.Param("pod")
	cluster := c.DefaultQuery("cluster", "")
	namespace := c.DefaultQuery("namespace", "default")

	events, err := h.Engine.GetPodEvents(c.Request.Context(), cluster, namespace, pod)
	if err != nil {
		response.Internal(c, "获取 Pod Event 失败: "+err.Error())
		return
	}

	response.OK(c, events)
}

// restartRequest 重启 Pod 请求体。
type restartRequest struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Confirm   bool   `json:"confirm"`
}

// RestartPod 处理 POST /api/v1/automation/pods/:pod/restart
func (h *AutomationHandler) RestartPod(c *gin.Context) {
	if h.Engine == nil {
		response.Internal(c, "自动化引擎未初始化")
		return
	}

	pod := c.Param("pod")
	var req restartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	if err := h.Engine.RestartPod(c.Request.Context(), req.Cluster, req.Namespace, pod, req.Confirm); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "Pod 重启指令已发送", "pod": pod, "namespace": req.Namespace})
}

// scaleRequest 扩容请求体。
type scaleRequest struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	Confirm   bool   `json:"confirm"`
}

// ScaleDeployment 处理 POST /api/v1/automation/deployments/:name/scale
func (h *AutomationHandler) ScaleDeployment(c *gin.Context) {
	if h.Engine == nil {
		response.Internal(c, "自动化引擎未初始化")
		return
	}

	name := c.Param("name")
	var req scaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas < 0 {
		response.BadRequest(c, "replicas 不能为负数")
		return
	}

	if err := h.Engine.ScaleDeployment(c.Request.Context(), req.Cluster, req.Namespace, name, req.Replicas, req.Confirm); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "Deployment 扩容指令已发送", "deployment": name, "replicas": req.Replicas})
}
