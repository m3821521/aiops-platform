package signals

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/rca"
)

// AlertLister 是 Alert 查询接口。
// 真实实现为 *alert.Repository。定义在 signals 包内便于测试 mock。
type AlertLister interface {
	List(ctx context.Context, filter alert.ListFilter, page, pageSize int) ([]alert.Alert, int64, error)
}

// AlertSignalCollector 从 Alertmanager 采集 Service Health Signals。
//
// 支持的 Signal 类型：
//   - alert_firing: 当前 firing 的告警
//
// 原则：
//   - Alert 只能作为 context 或 corroborating evidence，不能作为 direct_causal。
//   - Alert ≠ Root Cause。
//   - resolved alerts 不产生 active signal。
//   - 无 service label 时通过 pod + Service selector 映射。
type AlertSignalCollector struct {
	alerts AlertLister
}

// NewAlertSignalCollector 创建 AlertSignalCollector。
func NewAlertSignalCollector(alerts AlertLister) *AlertSignalCollector {
	return &AlertSignalCollector{alerts: alerts}
}

// Source 实现 SignalCollector 接口。
func (c *AlertSignalCollector) Source() string { return "alertmanager" }

// Collect 实现 SignalCollector 接口。
// 查询 namespace 下的 firing alerts，按 service/pod 过滤并映射到 Service。
func (c *AlertSignalCollector) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error) {
	if c.alerts == nil {
		return nil, nil
	}

	fetchedAt := time.Now()

	// 构建 pod name 集合，用于过滤
	podSet := make(map[string]bool, len(pods))
	for _, p := range pods {
		podSet[p.Name] = true
	}

	// 查询 namespace 下的 firing alerts（使用大 pageSize 获取全部）
	filter := alert.ListFilter{
		Status:    alert.StatusFiring,
		Namespace: svc.Namespace,
	}
	alerts, _, err := c.alerts.List(ctx, filter, 1, 500)
	if err != nil {
		return nil, fmt.Errorf("alert list failed: %w", err)
	}

	var evidences []rca.Evidence
	for i := range alerts {
		a := &alerts[i]

		// 映射到 Service：优先 service 字段，其次 pod 字段
		matched := false
		if a.Service != "" && a.Service == svc.Name {
			matched = true
		} else if a.Pod != "" && podSet[a.Pod] {
			matched = true
		}

		if !matched {
			continue
		}

		// Alert 级别：critical → corroborating，warning/info → context
		level := rca.EvidenceLevelContext
		causal := "contextual"
		score := 0.1
		if a.Severity == alert.SeverityCritical {
			level = rca.EvidenceLevelCorroborating
			causal = "supporting"
			score = 0.3
		}

		evidences = append(evidences, rca.Evidence{
			ID:            fmt.Sprintf("alert-%s", a.Fingerprint),
			Type:          "alert_firing",
			Level:         level,
			Source:        "alertmanager",
			SourceType:    "provider",
			Timestamp:     a.StartsAt,
			ResourceType:  "alert",
			ResourceName:  a.Alertname,
			Namespace:     a.Namespace,
			Severity:      a.Severity,
			Description:   fmt.Sprintf("Alert %s (%s) is firing: %s", a.Alertname, a.Severity, a.Instance),
			Score:         score,
			FetchedAt:     &fetchedAt,
			DataTimestamp: timePtr(a.StartsAt),
			TimestampAvailable: !a.StartsAt.IsZero(),
			TrustStatus:        "fresh",
			CausalRelevance:    causal,
		})
	}

	return evidences, nil
}
