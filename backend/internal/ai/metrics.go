package ai

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// AI Metrics
var (
	AIRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_requests_total",
		Help: "Total number of AI requests",
	}, []string{"provider", "model", "status"})

	AIRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_request_duration_seconds",
		Help:    "AI request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider", "model"})

	AIRequestErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_request_errors_total",
		Help: "Total number of AI request errors",
	}, []string{"provider", "model", "error_type"})

	AIToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_tool_calls_total",
		Help: "Total number of AI tool calls",
	}, []string{"tool_name", "status"})

	AIConversationMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_conversation_messages_total",
		Help: "Total number of AI conversation messages",
	}, []string{"role"})
)

// RecordAIRequest 记录 AI 请求指标。
func RecordAIRequest(provider, model, status string, duration float64) {
	AIRequestsTotal.WithLabelValues(provider, model, status).Inc()
	AIRequestDuration.WithLabelValues(provider, model).Observe(duration)
}

// RecordAIError 记录 AI 错误指标。
func RecordAIError(provider, model, errorType string) {
	AIRequestErrors.WithLabelValues(provider, model, errorType).Inc()
}

// RecordToolCall 记录工具调用指标。
func RecordToolCall(toolName, status string) {
	AIToolCallsTotal.WithLabelValues(toolName, status).Inc()
}
