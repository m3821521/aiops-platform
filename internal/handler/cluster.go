package handler

import (
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
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToNodeViews(items))
}

func (h *ClusterHandler) ListNamespaces(c *gin.Context) {
	items, err := h.Service.ListNamespaces(c.Request.Context(), clusterName(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToNamespaceViews(items))
}

func (h *ClusterHandler) ListPods(c *gin.Context) {
	items, err := h.Service.ListPods(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToPodViews(items))
}

func (h *ClusterHandler) ListDeployments(c *gin.Context) {
	items, err := h.Service.ListDeployments(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToDeploymentViews(items))
}

func (h *ClusterHandler) ListStatefulSets(c *gin.Context) {
	items, err := h.Service.ListStatefulSets(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToStatefulSetViews(items))
}

func (h *ClusterHandler) ListDaemonSets(c *gin.Context) {
	items, err := h.Service.ListDaemonSets(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToDaemonSetViews(items))
}

func (h *ClusterHandler) ListServices(c *gin.Context) {
	items, err := h.Service.ListServices(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToServiceViews(items))
}

func (h *ClusterHandler) ListConfigMaps(c *gin.Context) {
	items, err := h.Service.ListConfigMaps(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToConfigMapViews(items))
}

func (h *ClusterHandler) ListSecrets(c *gin.Context) {
	items, err := h.Service.ListSecrets(c.Request.Context(), clusterName(c), namespace(c))
	if err != nil {
		response.Internal(c, err.Error())
		return
	}
	response.OK(c, cluster.ToSecretViews(items))
}
