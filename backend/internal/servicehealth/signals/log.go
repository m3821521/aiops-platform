package signals

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/rca"
)

// LogSearcher 是 Elasticsearch 日志查询接口。
// 真实实现为 *logging.Client。定义在 signals 包内便于测试 mock。
type LogSearcher interface {
	Search(ctx context.Context, q logging.SearchQuery) (*logging.SearchResult, error)
}

// LogSignalCollector 从 Elasticsearch 采集 Service Health Signals。
//
// 支持的 Signal 类型：
//   - log_error_rate: 错误日志数量/速率
//
// 原则：
//   - 一次 namespace 范围查询 + Go 内存聚合（按 pod 分组），避免 N+1。
//   - ES 未配置 → empty（不是 error）。
//   - ES 查询失败 → error（不产生 error_rate=0 fake signal）。
//   - 无 error logs → empty（不是 error）。
type LogSignalCollector struct {
	es LogSearcher
	// timeWindow 是查询的时间窗口（默认 5 分钟）。
	timeWindow time.Duration
}

// NewLogSignalCollector 创建 LogSignalCollector。
// timeWindow 默认为 5 分钟（<=0 时使用默认值）。
func NewLogSignalCollector(es LogSearcher, timeWindow time.Duration) *LogSignalCollector {
	if timeWindow <= 0 {
		timeWindow = 5 * time.Minute
	}
	return &LogSignalCollector{es: es, timeWindow: timeWindow}
}

// Source 实现 SignalCollector 接口。
func (c *LogSignalCollector) Source() string { return "elasticsearch" }

// Collect 实现 SignalCollector 接口。
// 查询 namespace 下最近 timeWindow 的 error logs，按 pod 分组并映射到 Service。
func (c *LogSignalCollector) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error) {
	if c.es == nil {
		// ES 未配置 → empty（不是 error）
		return nil, nil
	}

	fetchedAt := time.Now()

	// 构建 pod name 集合，用于过滤
	podSet := make(map[string]bool, len(pods))
	for _, p := range pods {
		podSet[p.Name] = true
	}

	// 查询 namespace 下最近 timeWindow 的 error logs
	query := logging.SearchQuery{
		Namespace: svc.Namespace,
		Level:     "error",
		Start:     fetchedAt.Add(-c.timeWindow),
		End:       fetchedAt,
		Size:      200, // 获取足够的 hits 用于按 pod 分组
	}
	result, err := c.es.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch search failed: %w", err)
	}
	if result == nil || result.Total == 0 {
		// 无 error logs → empty（不是 error）
		return nil, nil
	}

	// 按 pod 分组统计 error log 数量
	podErrorCount := make(map[string]int)
	var latestTimestamp time.Time
	for _, hit := range result.Hits {
		if hit.Pod == "" || !podSet[hit.Pod] {
			continue
		}
		podErrorCount[hit.Pod]++
		if hit.Timestamp.After(latestTimestamp) {
			latestTimestamp = hit.Timestamp
		}
	}

	if len(podErrorCount) == 0 {
		// 有 error logs 但不属于当前 Service 的 pods → empty
		return nil, nil
	}

	var evidences []rca.Evidence
	for pod, count := range podErrorCount {
		// error log 数量 > 0 时标记为 corroborating
		level := rca.EvidenceLevelCorroborating
		causal := "supporting"
		score := 0.3

		evidences = append(evidences, rca.Evidence{
			ID:            fmt.Sprintf("log-error-%s-%s-%s-%d", svc.Cluster, svc.Namespace, pod, fetchedAt.Unix()),
			Type:          "log_error_rate",
			Level:         level,
			Source:        "elasticsearch",
			SourceType:    "provider",
			Timestamp:     latestTimestamp,
			ResourceType:  "pod",
			ResourceName:  pod,
			Namespace:     svc.Namespace,
			Value:         float64(count),
			Description:   fmt.Sprintf("Pod %s has %d error logs in last %s", pod, count, c.timeWindow),
			Score:         score,
			FetchedAt:     &fetchedAt,
			DataTimestamp: timePtr(latestTimestamp),
			TimestampAvailable: !latestTimestamp.IsZero(),
			TrustStatus:        "fresh",
			CausalRelevance:    causal,
		})
	}

	return evidences, nil
}
