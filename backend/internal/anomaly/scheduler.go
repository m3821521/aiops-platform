package anomaly

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Scheduler 是异常检测定时调度器。
// 按规则配置的间隔定期从 Prometheus 拉取指标，执行检测，保存结果，严重异常进入 Incident。
type Scheduler struct {
	service *Service
	rules   []AnomalyRule
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
}

// NewScheduler 创建调度器。
func NewScheduler(service *Service, rules []AnomalyRule) *Scheduler {
	return &Scheduler{
		service: service,
		rules:   rules,
		stopCh:  make(chan struct{}),
	}
}

// Start 启动调度器（非阻塞）。
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	slog.Info("anomaly scheduler started", "rules", len(s.rules))
	for _, rule := range s.rules {
		if !rule.Enabled {
			continue
		}
		s.wg.Add(1)
		go s.runRule(ctx, rule)
	}
}

// Stop 停止调度器。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	slog.Info("anomaly scheduler stopped")
}

// runRule 执行单条规则的定时检测。
func (s *Scheduler) runRule(ctx context.Context, rule AnomalyRule) {
	defer s.wg.Done()

	interval := rule.Interval
	if interval <= 0 {
		interval = 1 * time.Minute
	}

	// 启动时立即执行一次。
	s.executeRule(ctx, rule)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.executeRule(ctx, rule)
		}
	}
}

// executeRule 执行一次检测规则。
func (s *Scheduler) executeRule(ctx context.Context, rule AnomalyRule) {
	detectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := s.service.DetectAndPersist(detectCtx, DetectRequest{
		Query:      rule.Metric,
		Start:      time.Now().Add(-15 * time.Minute),
		End:        time.Now(),
		Step:       30 * time.Second,
		Thresholds: parametersToThreshold(rule.Parameters),
		ResourceType: rule.ResourceType,
		Namespace:    rule.Namespace,
		Cluster:      rule.Cluster,
		RuleName:     rule.Name,
	})
	if err != nil {
		slog.Warn("anomaly rule execution failed",
			"rule", rule.Name, "err", err)
		return
	}

	if len(result.Anomalies) > 0 {
		slog.Info("anomaly detected",
			"rule", rule.Name, "metric", result.Metric, "count", len(result.Anomalies))
	}
}

// parametersToThreshold 从规则参数构建阈值配置。
func parametersToThreshold(params map[string]any) ThresholdConfig {
	cfg := ThresholdConfig{}
	if v, ok := params["upper_warning"].(float64); ok {
		cfg.UpperWarning = &v
	}
	if v, ok := params["upper_critical"].(float64); ok {
		cfg.UpperCritical = &v
	}
	if v, ok := params["lower_warning"].(float64); ok {
		cfg.LowerWarning = &v
	}
	if v, ok := params["lower_critical"].(float64); ok {
		cfg.LowerCritical = &v
	}
	return cfg
}
