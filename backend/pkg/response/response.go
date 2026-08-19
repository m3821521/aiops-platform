package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 是统一返回格式，前端只需要认 code / message / data。
// request_id 用于链路追踪，便于用户报障时定位日志。
type Body struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
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

func Fail(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Body{Code: code, Message: message, RequestID: getRequestID(c)})
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

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Body{Code: 0, Message: "created", Data: data, RequestID: getRequestID(c)})
}
