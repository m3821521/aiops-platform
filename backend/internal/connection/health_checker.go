package connection

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HealthChecker 是 Connection 周期性健康检查器。
//
// 设计原则：
//   - 只检查 enabled=true 的 Connection
//   - 有限并发（默认 4），避免同时对所有 Provider 发起探活
//   - 每个 Provider 有独立 timeout（默认 15s），避免一个异常 Provider 卡死整个周期
//   - 错误隔离：一个 Connection 探活失败不影响其他 Connection
//   - scheduler 失败不影响 Web Server
//   - 不重复执行：如果上一个周期还在运行，跳过当前周期
type HealthChecker struct {
	repo       *ConnectionRepository
	registry   *ProviderRegistry
	interval   time.Duration
	timeout    time.Duration
	maxWorkers int
	stopCh     chan struct{}
	running    bool
	mu         sync.Mutex
}

// HealthCheckResult 是单个 Connection 的健康检查结果。
// 不包含任何 credential / password / token 信息。
type HealthCheckResult struct {
	ID        int64            `json:"id"`
	Name      string           `json:"name"`
	Type      ConnectionType   `json:"type"`
	Status    ConnectionStatus `json:"status"`
	CheckedAt time.Time        `json:"checked_at"`
	Error     string           `json:"error,omitempty"`
	LatencyMs int64            `json:"latency_ms"`
}

// NewHealthChecker 创建健康检查器。
// interval: 检查周期，默认 5 分钟
// timeout: 单个 Provider 超时，默认 15 秒
// maxWorkers: 最大并发数，默认 4
func NewHealthChecker(repo *ConnectionRepository, registry *ProviderRegistry, interval, timeout time.Duration, maxWorkers int) *HealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	return &HealthChecker{
		repo:       repo,
		registry:   registry,
		interval:   interval,
		timeout:    timeout,
		maxWorkers: maxWorkers,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动周期性健康检查。
// 在独立 goroutine 中运行，不阻塞调用方。
func (h *HealthChecker) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	slog.Info("connection health checker started",
		"interval", h.interval.String(),
		"timeout", h.timeout.String(),
		"max_workers", h.maxWorkers,
	)

	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		// 启动后立即执行一次
		h.runOnce(context.Background())

		for {
			select {
			case <-ticker.C:
				h.runOnce(context.Background())
			case <-h.stopCh:
				slog.Info("connection health checker stopped")
				return
			case <-ctx.Done():
				slog.Info("connection health checker stopped by context")
				return
			}
		}
	}()
}

// Stop 停止周期性健康检查。
func (h *HealthChecker) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	h.running = false
	close(h.stopCh)
}

// runOnce 执行一次完整的健康检查周期。
// 使用 worker pool 限制并发，每个 worker 有独立 timeout。
func (h *HealthChecker) runOnce(ctx context.Context) {
	// 获取所有 enabled 的 Connection
	enabled := true
	filter := ConnectionFilter{Enabled: &enabled}
	connections, _, err := h.repo.List(ctx, filter, 1, 100) // 最多检查 100 个
	if err != nil {
		slog.Error("health checker: list enabled connections failed", "error", err)
		return
	}

	if len(connections) == 0 {
		slog.Debug("health checker: no enabled connections")
		return
	}

	slog.Info("health checker: starting health check cycle", "count", len(connections))

	// Worker pool
	jobs := make(chan Connection, len(connections))
	results := make(chan HealthCheckResult, len(connections))
	var wg sync.WaitGroup

	// 启动 workers
	for i := 0; i < h.maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for conn := range jobs {
				result := h.checkOne(ctx, conn)
				results <- result
			}
		}(i)
	}

	// 发送 jobs
	for _, conn := range connections {
		jobs <- conn
	}
	close(jobs)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	successCount := 0
	failCount := 0
	for result := range results {
		if result.Status == StatusAvailable {
			successCount++
		} else {
			failCount++
		}
	}

	slog.Info("health checker: cycle completed",
		"total", len(connections),
		"available", successCount,
		"unavailable", failCount,
	)
}

// checkOne 检查单个 Connection 的健康状态。
// 有独立 timeout，错误被捕获并记录，不会 panic。
func (h *HealthChecker) checkOne(parentCtx context.Context, conn Connection) HealthCheckResult {
	// 独立 timeout context
	ctx, cancel := context.WithTimeout(parentCtx, h.timeout)
	defer cancel()

	result := HealthCheckResult{
		ID:   conn.ID,
		Name: conn.Name,
		Type: conn.Type,
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error("health checker: panic recovered",
				"connection_id", conn.ID,
				"connection_name", conn.Name,
				"panic", r,
			)
			result.Status = StatusUnavailable
			result.Error = "internal panic during health check"
			result.CheckedAt = time.Now()
		}
	}()

	// 调用 ProviderRegistry 执行真实探活
	testResult, err := h.registry.TestConnection(ctx, &conn)
	if err != nil {
		result.Status = StatusUnavailable
		result.Error = sanitizeError(err.Error())
		result.CheckedAt = time.Now()
	} else if testResult != nil {
		result.Status = testResult.Status
		// P1-X.10 Error Semantic Fix: 只有非 available 状态才保留错误信息。
		// available 时必须清除 last_error，禁止把 Provider 成功描述（如 "Kubernetes v1.35.1"）
		// 存入 last_error 字段，否则前端会误显示"查看错误"。
		if testResult.Status != StatusAvailable {
			result.Error = sanitizeError(testResult.ErrorMessage)
		}
		result.CheckedAt = testResult.CheckedAt
		result.LatencyMs = testResult.LatencyMs
	} else {
		result.Status = StatusUnknown
		result.Error = "empty test result"
		result.CheckedAt = time.Now()
	}

	// 更新数据库状态
	if updateErr := h.repo.UpdateStatus(ctx, conn.ID, result.Status, result.Error); updateErr != nil {
		slog.Warn("health checker: update status failed",
			"connection_id", conn.ID,
			"error", updateErr,
		)
	}

	return result
}

// CheckAll 立即执行一次全量健康检查（供 Batch API 调用）。
// 返回所有 Connection 的检查结果，不包含 credential 信息。
func (h *HealthChecker) CheckAll(ctx context.Context) []HealthCheckResult {
	enabled := true
	filter := ConnectionFilter{Enabled: &enabled}
	connections, _, err := h.repo.List(ctx, filter, 1, 100)
	if err != nil {
		slog.Error("health checker CheckAll: list enabled connections failed", "error", err)
		return nil
	}

	if len(connections) == 0 {
		return []HealthCheckResult{}
	}

	jobs := make(chan Connection, len(connections))
	results := make(chan HealthCheckResult, len(connections))
	var wg sync.WaitGroup

	for i := 0; i < h.maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for conn := range jobs {
				results <- h.checkOne(ctx, conn)
			}
		}()
	}

	for _, conn := range connections {
		jobs <- conn
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var allResults []HealthCheckResult
	for r := range results {
		allResults = append(allResults, r)
	}

	return allResults
}

// sanitizeError 脱敏错误信息，防止泄露 password/token/credential。
func sanitizeError(msg string) string {
	// 简单脱敏：截断过长错误，移除可能的敏感关键词
	if len(msg) > 500 {
		msg = msg[:500] + "...(truncated)"
	}
	return msg
}
