package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Body 是统一返回格式，前端只需要认 code / message / data。
// request_id 用于链路追踪，便于用户报障时定位日志。
// meta 是可选的元数据（如 provenance），旧前端不使用时可忽略。
type Body struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Meta      *Meta       `json:"meta,omitempty"`
}

// Meta 是响应元数据，当前仅包含 provenance。
type Meta struct {
	Provenance *Provenance `json:"provenance,omitempty"`
}

// Provenance 描述数据的来源和时间戳信息。
// 所有字段都是可选的，无法真实获得时为 nil / false。
// 绝对禁止伪造时间戳。
type Provenance struct {
	// Source 数据源标识，如 "kubernetes", "prometheus", "mysql", "topology"
	Source string `json:"source"`
	// SourceType 数据源类型，如 "provider", "mysql", "redis-cache", "kubernetes+prometheus"
	SourceType string `json:"sourceType,omitempty"`
	// FetchedAt 后端实际完成数据获取/准备响应的时间（backend runtime）
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	// DataTimestamp 底层数据自身携带的真实时间戳（如 Prometheus sample timestamp）
	// 无法获得时为 nil，timestampAvailable=false
	DataTimestamp *time.Time `json:"dataTimestamp,omitempty"`
	// SourceUpdatedAt 底层业务源最后更新时间（如 MySQL max(updated_at)）
	SourceUpdatedAt *time.Time `json:"sourceUpdatedAt,omitempty"`
	// CacheHit 是否命中缓存（仅当真实命中 Redis cache 时为 true）
	CacheHit bool `json:"cacheHit"`
	// CacheCreatedAt 缓存创建时间，无法获得时为 nil（禁止伪造）
	CacheCreatedAt *time.Time `json:"cacheCreatedAt,omitempty"`
	// CacheExpiresAt 缓存过期时间，由 Redis TTL 推算（now + TTL）
	CacheExpiresAt *time.Time `json:"cacheExpiresAt,omitempty"`
	// TimestampAvailable 后端是否真的提供了 dataTimestamp
	TimestampAvailable bool `json:"timestampAvailable"`
	// TimestampSemantics 明确解释 timestamp 的含义，如 "latest_prometheus_sample_timestamp"
	TimestampSemantics string `json:"timestampSemantics,omitempty"`
}

// getRequestID 从 gin context 中提取 request_id（由 middleware.RequestID 设置）。
func getRequestID(c *gin.Context) string {
	if v, exists := c.Get("request_id"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "success", Data: data, RequestID: getRequestID(c)})
}

// OKWithMeta 返回带元数据的成功响应。
// 用于需要 provenance 的 API（如 Topology cache, Prometheus query）。
func OKWithMeta(c *gin.Context, data interface{}, meta *Meta) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "success", Data: data, RequestID: getRequestID(c), Meta: meta})
}

// OKWithProvenance 便捷方法：直接传入 provenance，自动包装为 Meta。
func OKWithProvenance(c *gin.Context, data interface{}, prov *Provenance) {
	OKWithMeta(c, data, &Meta{Provenance: prov})
}

func Fail(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Body{Code: code, Message: message, RequestID: getRequestID(c)})
}

// FailWithMeta 返回带元数据的错误响应。
// 用于需要 provenance 的 API 失败场景（如 Prometheus 查询失败时仍需告知数据源和请求时间）。
// 错误响应必须保持原 HTTP 状态码，禁止把 500 改成 200。
func FailWithMeta(c *gin.Context, httpStatus int, code int, message string, meta *Meta) {
	c.JSON(httpStatus, Body{Code: code, Message: message, RequestID: getRequestID(c), Meta: meta})
}

// InternalWithProvenance 便捷方法：返回 500 + provenance 的错误响应。
// 用于 Prometheus / 外部 Provider 查询失败场景，明确告知 source 和 fetchedAt。
func InternalWithProvenance(c *gin.Context, message string, prov *Provenance) {
	FailWithMeta(c, http.StatusInternalServerError, 500, message, &Meta{Provenance: prov})
}

func BadRequest(c *gin.Context, message string) {
	Fail(c, http.StatusBadRequest, 400, message)
}

func NotFound(c *gin.Context, message string) {
	Fail(c, http.StatusNotFound, 404, message)
}

func Internal(c *gin.Context, message string) {
	// 内部错误不暴露具体原因，只返回通用消息。
	// 详细错误应在日志中记录（handler 层 slog.Error）。
	Fail(c, http.StatusInternalServerError, 500, message)
}

func Unauthorized(c *gin.Context, message string) {
	Fail(c, http.StatusUnauthorized, 401, message)
}

func Forbidden(c *gin.Context, message string) {
	Fail(c, http.StatusForbidden, 403, message)
}

func Conflict(c *gin.Context, message string) {
	Fail(c, http.StatusConflict, 409, message)
}

func TooManyRequests(c *gin.Context, message string) {
	Fail(c, http.StatusTooManyRequests, 429, message)
}

func ServiceUnavailable(c *gin.Context, message string) {
	Fail(c, http.StatusServiceUnavailable, 503, message)
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Body{Code: 0, Message: "created", Data: data, RequestID: getRequestID(c)})
}

// HasUpdatedAt 是一个约束，用于提取带有 UpdatedAt 字段的模型的最大更新时间。
type HasUpdatedAt interface {
	GetUpdatedAt() time.Time
}

// MaxUpdatedAt 从切片中提取最大的 UpdatedAt。
// 用于 MySQL 业务数据的 sourceUpdatedAt provenance。
// 返回 (maxTime, found)：found=false 表示切片为空。
func MaxUpdatedAt[T HasUpdatedAt](items []T) (time.Time, bool) {
	var max time.Time
	found := false
	for _, item := range items {
		t := item.GetUpdatedAt()
		if !found || t.After(max) {
			max = t
			found = true
		}
	}
	return max, found
}
