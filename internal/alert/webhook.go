package alert

import (
	"time"
)

// WebhookPayload 是 Alertmanager 发送的 webhook 请求体。
// 参考：https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type WebhookPayload struct {
	Receiver string         `json:"receiver"`
	Status   string         `json:"status"` // firing / resolved
	Alerts   []WebhookAlert `json:"alerts"`
}

// WebhookAlert 是单条告警。
type WebhookAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// ToAlert 将 Alertmanager 告警转换为数据库模型。
// 常用 label（alertname/severity/instance/pod/namespace/service/node）提取到独立字段，
// 完整 labels 和 annotations 存 JSON 字段。
func (w WebhookAlert) ToAlert() Alert {
	a := Alert{
		Fingerprint: w.Fingerprint,
		Alertname:   w.Labels["alertname"],
		Severity:    w.Labels["severity"],
		Status:      w.Status,
		Instance:    w.Labels["instance"],
		Pod:         w.Labels["pod"],
		Namespace:   w.Labels["namespace"],
		Service:     w.Labels["service"],
		Node:        w.Labels["node"],
		Labels:      w.Labels,
		Annotations: w.Annotations,
		StartsAt:    w.StartsAt,
	}

	// 只有 resolved 状态且 EndsAt 非零值时才填写结束时间。
	if w.Status == StatusResolved && !w.EndsAt.IsZero() {
		endsAt := w.EndsAt
		a.EndsAt = &endsAt
	}

	// 级别默认值。
	if a.Severity == "" {
		a.Severity = SeverityWarning
	}
	// 状态默认值。
	if a.Status == "" {
		a.Status = StatusFiring
	}
	return a
}
