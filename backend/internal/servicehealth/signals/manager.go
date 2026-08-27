package signals

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/rca"
)

// SignalCollectorManager 是 Signal Collector 的编排器。
//
// 职责：
//  1. 并行执行所有 SignalCollector（bounded concurrency）
//  2. 聚合所有成功采集到的 Evidence
//  3. 记录失败的数据源到 source_errors
//  4. 判断 partial failure / all failure / no evidence
//
// 原则：
//   - 部分数据源失败不阻塞其他数据源（partial failure）。
//   - 所有数据源都失败 → 返回 error（由调用方决定 HTTP 503）。
//   - 所有数据源成功但无数据 → 返回空 signals（empty，不是 error）。
//   - 使用标准库 channel 实现 bounded concurrency，不引入 x/sync 依赖。
type SignalCollectorManager struct {
	collectors []SignalCollector
	// maxConcurrency 是最大并发数，默认等于 collector 数量（<=0 时使用默认值）。
	maxConcurrency int
}

// NewSignalCollectorManager 创建 SignalCollectorManager。
// maxConcurrency <= 0 时使用默认值（等于 collector 数量，最多 5）。
func NewSignalCollectorManager(collectors []SignalCollector, maxConcurrency int) *SignalCollectorManager {
	if maxConcurrency <= 0 {
		maxConcurrency = len(collectors)
		if maxConcurrency > 5 {
			maxConcurrency = 5
		}
		if maxConcurrency < 1 {
			maxConcurrency = 1
		}
	}
	return &SignalCollectorManager{collectors: collectors, maxConcurrency: maxConcurrency}
}

// Collect 并行执行所有 Collector，聚合结果。
//
// 返回：
//   - result: 聚合结果（signals + source_errors + fetchedAt）
//   - err: 所有 Collector 都失败时返回非 nil error
//
// 部分失败时 result 包含成功的 signals 和 source_errors，err 为 nil。
func (m *SignalCollectorManager) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) (*SignalCollectionResult, error) {
	fetchedAt := time.Now()
	result := &SignalCollectionResult{
		Signals:      make([]rca.Evidence, 0),
		SourceErrors: make(map[string]string),
		FetchedAt:    fetchedAt,
	}

	if len(m.collectors) == 0 {
		return result, nil
	}

	// bounded concurrency: 使用 buffered channel 作为 semaphore
	sem := make(chan struct{}, m.maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex // 保护 result.Signals 和 result.SourceErrors
	successCount := 0
	errorCount := 0

	for _, collector := range m.collectors {
		wg.Add(1)
		go func(c SignalCollector) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			evidences, err := c.Collect(ctx, svc, pods)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errorCount++
				result.SourceErrors[c.Source()] = err.Error()
				return
			}

			successCount++
			if len(evidences) > 0 {
				result.Signals = append(result.Signals, evidences...)
			}
		}(collector)
	}

	wg.Wait()

	// 所有 Collector 都失败 → 返回 error
	if errorCount > 0 && successCount == 0 {
		return result, ErrAllCollectorsFailed
	}

	// 如果没有 source_errors，清空 map（避免 JSON 输出空 map）
	if len(result.SourceErrors) == 0 {
		result.SourceErrors = nil
	}

	return result, nil
}

// CollectForService 是便捷方法：接收 Service 标识和 pods，内部构建 ServiceContext。
// 这是为了方便 servicehealth.Manager 调用，避免每次都手动构建 ServiceContext。
func (m *SignalCollectorManager) CollectForService(
	ctx context.Context,
	id int64,
	name, namespace, cluster, workloadType, workloadName string,
	workloadSelector map[string]string,
	pods []corev1.Pod,
) (*SignalCollectionResult, error) {
	svc := ServiceContext{
		ID:               id,
		Name:             name,
		Namespace:        namespace,
		Cluster:          cluster,
		WorkloadType:     workloadType,
		WorkloadName:     workloadName,
		WorkloadSelector: workloadSelector,
	}
	return m.Collect(ctx, svc, pods)
}
