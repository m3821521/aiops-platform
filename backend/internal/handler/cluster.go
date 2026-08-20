package handler

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

type ClusterHandler struct {
	Service *cluster.Service
}

func clusterName(c *gin.Context) string {
	return c.Query("cluster")
}

func namespace(c *gin.Context) string {
	return cluster.NamespaceOrAll(c.Query("namespace"))
}

func (h *ClusterHandler) ListClusters(c *gin.Context) {
	response.OK(c, cluster.ToClusterViews(h.Service.Clusters()))
}

func (h *ClusterHandler) ListNodes(c *gin.Context) {
	items, err := h.Service.ListNodes(c.Request.Context(), clusterName(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToNodeViews(items))
}

func (h *ClusterHandler) ListNamespaces(c *gin.Context) {
	items, err := h.Service.ListNamespaces(c.Request.Context(), clusterName(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToNamespaceViews(items))
}

func (h *ClusterHandler) ListPods(c *gin.Context) {
	items, err := h.Service.ListPods(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToPodViews(items))
}

func (h *ClusterHandler) ListDeployments(c *gin.Context) {
	items, err := h.Service.ListDeployments(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToDeploymentViews(items))
}

func (h *ClusterHandler) ListStatefulSets(c *gin.Context) {
	items, err := h.Service.ListStatefulSets(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToStatefulSetViews(items))
}

func (h *ClusterHandler) ListDaemonSets(c *gin.Context) {
	items, err := h.Service.ListDaemonSets(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToDaemonSetViews(items))
}

func (h *ClusterHandler) ListServices(c *gin.Context) {
	items, err := h.Service.ListServices(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToServiceViews(items))
}

func (h *ClusterHandler) ListConfigMaps(c *gin.Context) {
	items, err := h.Service.ListConfigMaps(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToConfigMapViews(items))
}

func (h *ClusterHandler) ListSecrets(c *gin.Context) {
	items, err := h.Service.ListSecrets(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "Kubernetes 服务不可用: "+err.Error())
		return
	}
	response.OK(c, cluster.ToSecretViews(items))
}

// GetPod 获取 Pod 详情
func (h *ClusterHandler) GetPod(c *gin.Context) {
	name := c.Param("name")
	ns := namespace(c)
	pod, err := h.Service.GetPod(c.Request.Context(), clusterName(c), ns, name)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	view := cluster.ToPodViews([]corev1.Pod{*pod})[0]
	yaml, _ := h.Service.GetPodYAML(c.Request.Context(), clusterName(c), ns, name)
	response.OK(c, cluster.PodDetail{PodView: view, YAML: yaml})
}

// GetNode 获取 Node 详情
func (h *ClusterHandler) GetNode(c *gin.Context) {
	name := c.Param("name")
	node, err := h.Service.GetNode(c.Request.Context(), clusterName(c), name)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, cluster.ToNodeDetail(node))
}

// GetDeployment 获取 Deployment 详情
func (h *ClusterHandler) GetDeployment(c *gin.Context) {
	name := c.Param("name")
	dep, err := h.Service.GetDeployment(c.Request.Context(), clusterName(c), namespace(c), name)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	view := cluster.ToDeploymentViews([]appsv1.Deployment{*dep})[0]
	response.OK(c, view)
}

// GetNodeMetrics 获取节点 CPU/内存指标（通过 metrics-server）
func (h *ClusterHandler) GetNodeMetrics(c *gin.Context) {
	metrics, err := h.Service.GetNodeMetrics(c.Request.Context(), clusterName(c))
	if err != nil {
		response.ServiceUnavailable(c, "获取节点指标失败: "+err.Error())
		return
	}
	response.OK(c, metrics)
}

// GetPodMetrics 获取 Pod CPU/内存指标（通过 metrics-server）
func (h *ClusterHandler) GetPodMetrics(c *gin.Context) {
	metrics, err := h.Service.GetPodMetrics(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.ServiceUnavailable(c, "获取 Pod 指标失败: "+err.Error())
		return
	}
	response.OK(c, metrics)
}
