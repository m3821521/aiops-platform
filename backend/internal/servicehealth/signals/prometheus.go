package signals

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/common/model"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/internal/rca"
)

// PromQuerier 是 Prometheus 查询接口。
// 真实实现为 monitoring.Querier（*monitoring.Client 或 *monitoring.CachedQuerier）。
// 定义在 signals 包内，避免直接依赖具体实现。
type PromQuerier interface {
	Query(ctx context.Context, query string, ts time.Time) (*monitoring.QueryResult, error)
}

// PrometheusSignalCollector 从 Prometheus 采集 Service Health Signals。
//
// 支持的 Signal 类型：
//   - prometheus_error_rate: HTTP 请求错误率
//   - prometheus_latency: 请求延迟（P95 或平均）
//
// 原则：
//   - 使用 batch PromQL（sum by (namespace, pod)），避免 N+1 查询。
//   - 通过 namespace + pod label 映射到 Service（不假设 service label 存在）。
//   - query error → error Evidence（不产生 value=0 fake signal）。
//   - empty series → 无 signal（empty，不是 error）。
//   - zero value → valid zero signal（不是 empty）。
type PrometheusSignalCollector struct {
	querier PromQuerier
	// staleThreshold 是 sample 数据年龄阈值，超过此值标记为 stale。
	staleThreshold time.Duration
}

// NewPrometheusSignalCollector 创建 PrometheusSignalCollector。
// staleThreshold 默认为 5 分钟（<=0 时使用默认值）。
func NewPrometheusSignalCollector(querier PromQuerier, staleThreshold time.Duration) *PrometheusSignalCollector {
	if staleThreshold <= 0 {
		staleThreshold = 5 * time.Minute
	}
	return &PrometheusSignalCollector{querier: querier, staleThreshold: staleThreshold}
}

// Source 实现 SignalCollector 接口。
func (c *PrometheusSignalCollector) Source() string { return "prometheus" }

// Collect 实现 SignalCollector 接口。
// 执行 batch PromQL 查询，解析结果并映射到 Service。
func (c *PrometheusSignalCollector) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error) {
	if c.querier == nil {
		// Prometheus 未配置 → empty（不是 error）
		return nil, nil
	}

	fetchedAt := time.Now()
	var evidences []rca.Evidence

	// 构建 pod name 集合，用于过滤查询结果
	podSet := make(map[string]bool, len(pods))
	for _, p := range pods {
		podSet[p.Name] = true
	}

	// 1. Error rate: sum by (namespace, pod) (rate(http_requests_total{status=~"5.."}[5m]))
	//    / sum by (namespace, pod) (rate(http_requests_total[5m]))
	//    使用通用 PromQL，兼容不同 exporter
	errorRateQuery := fmt.Sprintf(
		`sum by (namespace, pod) (rate(http_requests_total{namespace="%s",status=~"5.."}[5m])) / sum by (namespace, pod) (rate(http_requests_total{namespace="%s"}[5m]))`,
		svc.Namespace, svc.Namespace,
	)
	errorEvidences, err := c.queryAndParse(ctx, errorRateQuery, "prometheus_error_rate", svc, podSet, fetchedAt, 0.05)
	if err != nil {
		return nil, fmt.Errorf("prometheus error_rate query failed: %w", err)
	}
	evidences = append(evidences, errorEvidences...)

	// 2. Latency: histogram_quantile(0.95, sum by (namespace, pod, le) (rate(http_request_duration_seconds_bucket{namespace="..."}[5m])))
	latencyQuery := fmt.Sprintf(
		`histogram_quantile(0.95, sum by (namespace, pod, le) (rate(http_request_duration_seconds_bucket{namespace="%s"}[5m])))`,
		svc.Namespace,
	)
	latencyEvidences, err := c.queryAndParse(ctx, latencyQuery, "prometheus_latency", svc, podSet, fetchedAt, 1.0)
	if err != nil {
		return nil, fmt.Errorf("prometheus latency query failed: %w", err)
	}
	evidences = append(evidences, latencyEvidences...)

	return evidences, nil
}

// queryAndParse 执行 PromQL 查询并解析结果为 []rca.Evidence。
// threshold 是异常阈值（error_rate > 0.05 或 latency > 1.0s 时标记为 corroborating）。
func (c *PrometheusSignalCollector) queryAndParse(
	ctx context.Context,
	query, signalType string,
	svc ServiceContext,
	podSet map[string]bool,
	fetchedAt time.Time,
	threshold float64,
) ([]rca.Evidence, error) {
	result, err := c.querier.Query(ctx, query, time.Time{})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Result == nil {
		// empty series → 无 signal（empty，不是 error）
		return nil, nil
	}

	// 类型断言为 model.Vector（instant query 结果）
	vector, ok := result.Result.(model.Vector)
	if !ok {
		// 可能是 model.Matrix 或其他类型，尝试转换
		// 如果无法转换，返回 empty（不报错，因为查询成功了）
		return nil, nil
	}

	var evidences []rca.Evidence
	for _, sample := range vector {
		// 从 metric labels 中提取 namespace 和 pod
		namespace := string(sample.Metric[model.LabelName("namespace")])
		pod := string(sample.Metric[model.LabelName("pod")])

		// 过滤：只保留属于当前 Service 的 pod
		if namespace != svc.Namespace || !podSet[pod] {
			continue
		}

		value := float64(sample.Value)
		sampleTS := sample.Timestamp.Time()

		// 判断 trustStatus
		trustStatus := "fresh"
		if !sampleTS.IsZero() && fetchedAt.Sub(sampleTS) > c.staleThreshold {
			trustStatus = "stale"
		}

		// 判断 level 和 causalRelevance
		level := rca.EvidenceLevelContext
		causal := "contextual"
		score := 0.1
		if value > threshold {
			level = rca.EvidenceLevelCorroborating
			causal = "supporting"
			score = 0.4
		}

		evidences = append(evidences, rca.Evidence{
			ID:            fmt.Sprintf("%s-%s-%s-%s-%d", signalType, svc.Cluster, svc.Namespace, pod, sampleTS.Unix()),
			Type:          rca.EvidenceType(signalType),
			Level:         level,
			Source:        "prometheus",
			SourceType:    "provider",
			Timestamp:     sampleTS,
			ResourceType:  "pod",
			ResourceName:  pod,
			Namespace:     namespace,
			Metric:        query,
			Value:         value,
			Description:   fmt.Sprintf("%s for pod %s: %.4f", signalType, pod, value),
			Score:         score,
			FetchedAt:     &fetchedAt,
			DataTimestamp: timePtr(sampleTS),
			TimestampAvailable: !sampleTS.IsZero(),
			TrustStatus:        trustStatus,
			CausalRelevance:    causal,
		})
	}

	return evidences, nil
}
