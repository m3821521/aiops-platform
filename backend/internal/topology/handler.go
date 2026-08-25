package topology

import (
	"strconv"
	"time"

	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// Handler 处理拓扑相关 HTTP 请求。
type Handler struct {
	Service *Service
}

// GetGraph 处理 GET /api/v1/topology
// 查询参数：cluster, namespace, refresh
// 返回带 meta.provenance 的响应，真实描述 cache hit/miss 和缓存时间。
func (h *Handler) GetGraph(c *gin.Context) {
	if h.Service == nil {
		response.Internal(c, "topology service not initialized")
		return
	}

	cluster := c.DefaultQuery("cluster", "local")
	namespace := c.Query("namespace")
	refresh := c.Query("refresh") == "true"

	graph, cacheProv, err := h.Service.GetGraphWithProvenance(c.Request.Context(), cluster, namespace, refresh)
	if err != nil {
		response.ServiceUnavailable(c, "拓扑服务不可用（Kubernetes 连接失败）: "+err.Error())
		return
	}

	// 构建真实 provenance。
	fetchedAt := time.Now()
	sourceType := "kubernetes+prometheus"
	if cacheProv.Hit {
		sourceType = "redis-cache"
	}
	prov := &response.Provenance{
		Source:              "topology",
		SourceType:          sourceType,
		FetchedAt:           &fetchedAt,
		CacheHit:            cacheProv.Hit,
		CacheCreatedAt:      cacheProv.CreatedAt,
		CacheExpiresAt:      cacheProv.ExpiresAt,
		TimestampAvailable:  false,
		TimestampSemantics:  "topology graph has no intrinsic data timestamp; fetchedAt is backend acquisition time",
	}
	response.OKWithProvenance(c, graph, prov)
}

// GetNode 处理 GET /api/v1/topology/nodes/:type/:name
func (h *Handler) GetNode(c *gin.Context) {
	if h.Service == nil {
		response.Internal(c, "topology service not initialized")
		return
	}

	cluster := c.DefaultQuery("cluster", "local")
	namespace := c.Query("namespace")
	typ := ResourceType(c.Param("type"))
	name := c.Param("name")

	node, err := h.Service.GetNode(c.Request.Context(), cluster, typ, namespace, name)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, node)
}

// GetDependencies 处理 GET /api/v1/topology/dependencies/:type/:name
func (h *Handler) GetDependencies(c *gin.Context) {
	if h.Service == nil {
		response.Internal(c, "topology service not initialized")
		return
	}

	cluster := c.DefaultQuery("cluster", "local")
	namespace := c.Query("namespace")
	typ := ResourceType(c.Param("type"))
	name := c.Param("name")

	result, err := h.Service.GetDependencies(c.Request.Context(), cluster, typ, namespace, name)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, result)
}

// GetImpact 处理 GET /api/v1/topology/impact/:type/:name
func (h *Handler) GetImpact(c *gin.Context) {
	if h.Service == nil {
		response.Internal(c, "topology service not initialized")
		return
	}

	cluster := c.DefaultQuery("cluster", "local")
	namespace := c.Query("namespace")
	typ := ResourceType(c.Param("type"))
	name := c.Param("name")

	result, err := h.Service.GetImpact(c.Request.Context(), cluster, typ, namespace, name)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, result)
}

// InvalidateCache 处理 POST /api/v1/topology/cache/invalidate
func (h *Handler) InvalidateCache(c *gin.Context) {
	if h.Service == nil {
		response.OK(c, gin.H{"status": "skipped"})
		return
	}

	cluster := c.DefaultQuery("cluster", "local")
	namespace := c.Query("namespace")
	h.Service.InvalidateCache(c.Request.Context(), cluster, namespace)
	response.OK(c, gin.H{"status": "invalidated", "cluster": cluster, "namespace": namespace})
}

// parseID 辅助函数（预留）。
func parseID(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}
