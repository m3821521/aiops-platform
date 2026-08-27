package servicehealth

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/aiops/aiops-platform/internal/servicehealth/signals"
	"github.com/aiops/aiops-platform/pkg/response"
)

// Handler 提供 Service Health 的 HTTP API。
type Handler struct {
	svc *Manager
}

// NewHandler 创建 Service Health Handler。
func NewHandler(svc *Manager) *Handler {
	return &Handler{svc: svc}
}

// List GET /api/v1/platform/services
//
// Query:
//   - cluster (必填)
//   - namespace (可选)
//   - page (可选, 默认 1)
//   - page_size (可选, 默认 50)
//
// 每次 List 调用前自动执行 Sync（Discovery + Upsert），确保数据最新。
//
// Error != Empty:
//   - K8s API 失败 → HTTP 503（不返回空列表）
//   - DB 失败 → HTTP 500
//   - K8s 正常但无 Service → HTTP 200 + {items: [], total: 0}
func (h *Handler) List(c *gin.Context) {
	cluster := c.Query("cluster")
	if cluster == "" {
		response.BadRequest(c, "cluster query parameter is required")
		return
	}
	namespace := c.Query("namespace")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	filter := ListFilter{
		Cluster:   cluster,
		Namespace: namespace,
	}

	items, total, err := h.svc.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		// K8s API 不可用或 DB 错误，不返回空列表伪装成功
		response.ServiceUnavailable(c, "Service discovery failed: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Get GET /api/v1/platform/services/:id
//
// 按 ID 查询 Service 详情。不触发 Discovery。
func (h *Handler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid service id")
		return
	}

	svc, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Internal(c, "Failed to fetch service: "+err.Error())
		return
	}
	if svc == nil {
		response.NotFound(c, "service not found")
		return
	}

	response.OK(c, svc)
}

// Sync POST /api/v1/platform/services/sync
//
// 显式触发 Kubernetes Discovery + Upsert。
//
// Query:
//   - cluster (必填)
//   - namespace (可选)
//
// 返回 discovered 的 Service 数量。
func (h *Handler) Sync(c *gin.Context) {
	cluster := c.Query("cluster")
	if cluster == "" {
		response.BadRequest(c, "cluster query parameter is required")
		return
	}
	namespace := c.Query("namespace")

	count, err := h.svc.Sync(c.Request.Context(), cluster, namespace)
	if err != nil {
		response.ServiceUnavailable(c, "Service sync failed: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"discovered": count,
		"cluster":    cluster,
		"namespace":  namespace,
	})
}

// Signals GET /api/v1/platform/services/:id/signals
//
// 采集指定 Service 的 Health Signals。
//
// Query:
//   - cluster (必填) — P2-01 Phase 3 G5: Service ID 必须配合 cluster 校验
//
// 返回:
//   - signals: 成功采集到的 Evidence 列表
//   - source_errors: 失败的数据源及错误信息（partial failure 时）
//   - fetched_at: 采集时间
//
// Error != Empty:
//   - Service 不存在或 cluster 不匹配 → HTTP 404
//   - 所有 Collector 都失败 → HTTP 503（不返回空 signals 伪装成功）
//   - 部分 Collector 失败 → HTTP 200 + signals + source_errors
//   - 所有 Collector 成功但无数据 → HTTP 200 + signals=[]（empty，不是 error）
func (h *Handler) Signals(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid service id")
		return
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		response.BadRequest(c, "cluster query parameter is required")
		return
	}

	result, err := h.svc.CollectSignals(c.Request.Context(), id, cluster)
	if err != nil {
		// 所有 Collector 都失败 → 503
		if errors.Is(err, signals.ErrAllCollectorsFailed) {
			if result != nil {
				// 返回 source_errors 供前端展示
				response.ServiceUnavailable(c, "All signal collectors failed")
				return
			}
			response.ServiceUnavailable(c, "All signal collectors failed")
			return
		}
		// 其他错误（K8s API 失败、signalsManager 未配置等）→ 500
		response.Internal(c, "Failed to collect signals: "+err.Error())
		return
	}
	if result == nil {
		// Service 不存在或 cluster 不匹配 → 404
		response.NotFound(c, "service not found or cluster mismatch")
		return
	}

	response.OK(c, result)
}

// Health GET /api/v1/platform/services/:id/health
//
// 采集 Signals 并执行 Health Evaluation，返回组合结果。
//
// Query:
//   - cluster (必填) — P2-01 Phase 3 G5: Service ID 必须配合 cluster 校验
//
// 返回:
//   - service: Service 基本信息（id/name/namespace/cluster）
//   - health: Health Evaluation（state/reason/evidence_ids/evaluated_at）
//   - signals: Signal Trust 分布（total/fresh/stale/error/empty）
//   - source_errors: 失败的数据源及错误信息（partial failure 时）
//
// Error != Empty:
//   - cluster 缺失 → HTTP 400
//   - Service 不存在或 cluster 不匹配 → HTTP 404
//   - signalsManager 未配置 / K8s API 失败 → HTTP 500
//   - 所有 Collector 都失败 → HTTP 200 + health=unknown + source_errors（unknown 是有效状态）
//   - 部分 Collector 失败 → HTTP 200 + health（由 Evaluator 决定）+ source_errors
//
// 核心语义：
//   - Health State 完全来自 Phase 4 HealthEvaluator，不在 Handler 中重新判断
//   - no data != healthy
//   - source_errors 存在不强制改变 Health State
func (h *Handler) Health(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid service id")
		return
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		response.BadRequest(c, "cluster query parameter is required")
		return
	}

	result, err := h.svc.EvaluateHealth(c.Request.Context(), id, cluster)
	if err != nil {
		// signalsManager 未配置 / K8s API 失败 → 500
		response.Internal(c, "Failed to evaluate health: "+err.Error())
		return
	}
	if result == nil {
		// Service 不存在或 cluster 不匹配 → 404
		response.NotFound(c, "service not found or cluster mismatch")
		return
	}

	response.OK(c, result)
}
