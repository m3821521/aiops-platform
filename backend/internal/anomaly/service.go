package anomaly

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/prometheus/common/model"
)

// ThresholdConfig 静态阈值配置。
type ThresholdConfig struct {
	UpperWarning  *float64 `json:"upper_warning"`
	UpperCritical *float64 `json:"upper_critical"`
	LowerWarning  *float64 `json:"lower_warning"`
	LowerCritical *float64 `json:"lower_critical"`
}

// DetectRequest 异常检测请求。
type DetectRequest struct {
	Query        string          `json:"query"`
	Start        time.Time       `json:"start"`
	End          time.Time       `json:"end"`
	Step         time.Duration   `json:"step"`
	Thresholds   ThresholdConfig `json:"thresholds"`
	ResourceType string          `json:"resource_type,omitempty"`
	Namespace    string          `json:"namespace,omitempty"`
	Cluster      string          `json:"cluster,omitempty"`
	RuleName     string          `json:"rule_name,omitempty"`
}

// DetectResult 异常检测结果。
type DetectResult struct {
	Metric        string    `json:"metric"`
	PointsChecked int       `json:"points_checked"`
	Anomalies     []Anomaly `json:"anomalies"`
	SavedCount    int       `json:"saved_count,omitempty"`
}

// IncidentSink 是 Incident 服务的接口，用于异常信号进入 Incident。
// 通过接口避免 anomaly → incident 循环依赖。
type IncidentSink interface {
	// IngestSignal 接入一个信号，返回关联的 Incident ID。
	IngestAnomalySignal(ctx context.Context, sig AnomalySignal) (int64, error)
}

// AnomalySignal 是发送给 Incident 的异常信号。
type AnomalySignal struct {
	ID           int64
	Title        string
	Severity     string
	Cluster      string
	Namespace    string
	Service      string
	ResourceType string
	ResourceName string
	Timestamp    time.Time
	Resolved     bool
	Metric       string
	Value        float64
	AnomalyScore float64
	Reason       string
	Algorithm    string
}

// Service 异常检测服务，从 Prometheus 拉取数据并运行检测器。
type Service struct {
	querier      monitoring.Querier
	repo         *Repository
	incidentSink IncidentSink
}

// NewService 创建异常检测服务（不持久化，兼容旧用法）。
func NewService(querier monitoring.Querier) *Service {
	return &Service{querier: querier}
}

// NewServiceWithRepo 创建带持久化的异常检测服务。
func NewServiceWithRepo(querier monitoring.Querier, repo *Repository) *Service {
	return &Service{querier: querier, repo: repo}
}

// SetIncidentSink 设置 Incident 集成。
func (s *Service) SetIncidentSink(sink IncidentSink) {
	s.incidentSink = sink
}

// SetQuerier 替换 Prometheus Querier（用于 Provider 迁移）。
func (s *Service) SetQuerier(querier monitoring.Querier) {
	s.querier = querier
}

// Detect 执行异常检测（即时返回，不持久化）。
// 保留旧 API 兼容性。
func (s *Service) Detect(ctx context.Context, req DetectRequest) (*DetectResult, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("query 不能为空")
	}
	if req.Start.IsZero() || req.End.IsZero() {
		return nil, fmt.Errorf("start 和 end 不能为空")
	}
	if !req.End.After(req.Start) {
		return nil, fmt.Errorf("end 必须晚于 start")
	}
	if req.Step <= 0 {
		req.Step = time.Minute
	}

	result, err := s.querier.QueryRange(ctx, req.Query, req.Start, req.End, req.Step)
	if err != nil {
		return nil, fmt.Errorf("查询 Prometheus 失败: %w", err)
	}

	points, metricName, err := matrixToPoints(result)
	if err != nil {
		return nil, err
	}

	detector := NewStaticThresholdDetector(
		req.Thresholds.UpperWarning,
		req.Thresholds.UpperCritical,
		req.Thresholds.LowerWarning,
		req.Thresholds.LowerCritical,
	)

	anomalies := detector.Detect(metricName, points)

	return &DetectResult{
		Metric:        metricName,
		PointsChecked: len(points),
		Anomalies:     anomalies,
	}, nil
}

// DetectAndPersist 执行检测并持久化结果，严重异常进入 Incident。
// 支持 Prometheus 返回多序列（每个序列对应一个资源）。
func (s *Service) DetectAndPersist(ctx context.Context, req DetectRequest) (*DetectResult, error) {
	if s.querier == nil {
		return nil, fmt.Errorf("Prometheus monitoring data source unavailable")
	}
	if req.Query == "" {
		return nil, fmt.Errorf("query 不能为空")
	}
	if req.Step <= 0 {
		req.Step = 30 * time.Second
	}

	result, err := s.querier.QueryRange(ctx, req.Query, req.Start, req.End, req.Step)
	if err != nil {
		return nil, fmt.Errorf("查询 Prometheus 失败: %w", err)
	}

	matrix, ok := result.Result.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("查询结果不是 matrix 类型")
	}

	detector := NewStaticThresholdDetector(
		req.Thresholds.UpperWarning,
		req.Thresholds.UpperCritical,
		req.Thresholds.LowerWarning,
		req.Thresholds.LowerCritical,
	)

	totalPoints := 0
	totalAnomalies := 0
	savedCount := 0

	for _, stream := range matrix {
		metricName := string(stream.Metric["__name__"])
		if metricName == "" {
			metricName = req.RuleName
		}
		if metricName == "" {
			// Prometheus 计算表达式（如 100 - avg(...)）没有 __name__ 标签，
			// 且 RuleName 也为空时，使用查询表达式的摘要作为 metric 名。
			metricName = "computed_metric"
		}
		// 从标签提取资源信息。
		resourceName := string(stream.Metric["pod"])
		if resourceName == "" {
			resourceName = string(stream.Metric["instance"])
		}
		if resourceName == "" {
			resourceName = string(stream.Metric["node"])
		}
		namespace := string(stream.Metric["namespace"])
		if namespace == "" {
			namespace = req.Namespace
		}
		cluster := req.Cluster
		if cluster == "" {
			cluster = "local"
		}
		// Resource Type 自动识别：优先使用请求指定的，否则根据 Prometheus 标签推断。
		resourceType := req.ResourceType
		if resourceType == "" {
			resourceType = detectResourceType(stream.Metric)
		}

		points := make([]DataPoint, 0, len(stream.Values))
		for _, sp := range stream.Values {
			points = append(points, DataPoint{
				Timestamp: sp.Timestamp.Time(),
				Value:     float64(sp.Value),
			})
		}
		totalPoints += len(points)

		anomalies := detector.Detect(metricName, points)
		totalAnomalies += len(anomalies)

		if s.repo != nil {
			// 异常恢复：如果当前没有检测到异常，但之前有活跃异常，标记为已恢复。
			if len(anomalies) == 0 {
				if existing, err := s.repo.FindActiveByResourceAndMetric(ctx, cluster, resourceType, resourceName, metricName); err == nil && existing.ID > 0 {
					_ = s.repo.MarkResolved(ctx, existing.ID)
					slog.Info("anomaly recovered", "metric", metricName, "resource", resourceName)
				}
				continue
			}

			// 去重合并：同一资源+metric只保留最新一条异常，更新而不是新建。
			// 取最新的异常点（时间最大的）作为当前状态。
			latest := anomalies[len(anomalies)-1]
			for _, a := range anomalies {
				if a.Timestamp.After(latest.Timestamp) {
					latest = a
				}
			}

			rec := &AnomalyRecord{
				Metric:       metricName,
				ResourceType: resourceType,
				ResourceName: resourceName,
				Namespace:    namespace,
				Cluster:      cluster,
				Timestamp:    latest.Timestamp,
				Value:        latest.Value,
				AnomalyScore: latest.AnomalyScore,
				Severity:     latest.Severity,
				Algorithm:    latest.Detector,
				Reason:       latest.Reason,
				Status:       AnomalyStatusActive,
			}
			// 计算 baseline（简单取窗口均值）。
			if len(points) > 0 {
				sum := 0.0
				for _, p := range points {
					sum += p.Value
				}
				rec.Baseline = sum / float64(len(points))
			}
			if req.Thresholds.UpperWarning != nil {
				rec.ExpectedMax = *req.Thresholds.UpperWarning
			}

			// 去重：如果已有活跃异常，更新；否则新建。
			if existing, err := s.repo.FindActiveByResourceAndMetric(ctx, cluster, resourceType, resourceName, metricName); err == nil && existing.ID > 0 {
				existing.Value = rec.Value
				existing.AnomalyScore = rec.AnomalyScore
				existing.Severity = rec.Severity
				existing.Reason = rec.Reason
				existing.Timestamp = rec.Timestamp
				existing.Baseline = rec.Baseline
				existing.ExpectedMax = rec.ExpectedMax
				if err := s.repo.db.WithContext(ctx).Save(existing).Error; err != nil {
					slog.Warn("anomaly: update failed", "metric", metricName, "err", err)
				}
				rec = existing
			} else {
				if err := s.repo.db.WithContext(ctx).Create(rec).Error; err != nil {
					slog.Warn("anomaly: create failed", "metric", metricName, "err", err)
					continue
				}
				savedCount++
			}

			// 严重异常进入 Incident（仅新建时）。
			if s.incidentSink != nil && rec.IncidentID == nil && (latest.Severity == SeverityCritical || latest.Severity == SeverityWarning) {
				incidentID, err := s.incidentSink.IngestAnomalySignal(ctx, AnomalySignal{
					ID:           rec.ID,
					Title:        fmt.Sprintf("%s: %s", latest.Severity, metricName),
					Severity:     latest.Severity,
					Cluster:      cluster,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
					Timestamp:    latest.Timestamp,
					Metric:       metricName,
					Value:        latest.Value,
					AnomalyScore: latest.AnomalyScore,
					Reason:       latest.Reason,
					Algorithm:    latest.Detector,
				})
				if err == nil && incidentID > 0 {
					_ = s.repo.UpdateIncident(ctx, rec.ID, incidentID)
				}
			}
		}
	}

	return &DetectResult{
		Metric:        req.RuleName,
		PointsChecked: totalPoints,
		Anomalies:     nil, // 持久化模式下不返回全部异常
		SavedCount:    savedCount,
	}, nil
}

// matrixToPoints 将 Prometheus 查询结果转换为 DataPoint 切片（取第一条序列）。
func matrixToPoints(result *monitoring.QueryResult) ([]DataPoint, string, error) {
	if result == nil || result.Result == nil {
		return nil, "", nil
	}

	matrix, ok := result.Result.(model.Matrix)
	if !ok {
		return nil, "", fmt.Errorf("查询结果不是 matrix 类型，实际: %s", result.ResultType)
	}
	if len(matrix) == 0 {
		return nil, "", nil
	}

	stream := matrix[0]
	metricName := string(stream.Metric["__name__"])
	if metricName == "" {
		metricName = "metric"
	}

	points := make([]DataPoint, 0, len(stream.Values))
	for _, sp := range stream.Values {
		points = append(points, DataPoint{
			Timestamp: sp.Timestamp.Time(),
			Value:     float64(sp.Value),
		})
	}
	return points, metricName, nil
}

// detectResourceType 根据 Prometheus 标签自动推断资源类型。
// 优先级：pod > deployment > statefulset > daemonset > service > node > unknown
// 不默认 pod，避免 node-exporter 等节点指标被错误标记为 pod。
func detectResourceType(metric model.Metric) string {
	if string(metric["pod"]) != "" {
		return "pod"
	}
	if string(metric["deployment"]) != "" {
		return "deployment"
	}
	if string(metric["statefulset"]) != "" {
		return "statefulset"
	}
	if string(metric["daemonset"]) != "" {
		return "daemonset"
	}
	if string(metric["service"]) != "" {
		return "service"
	}
	if string(metric["node"]) != "" {
		return "node"
	}
	// 通过 job 名称推断：node-exporter / kube-state-metrics 等。
	job := string(metric["job"])
	if job != "" {
		if containsAny(job, "node", "node-exporter", "node_exporter") {
			return "node"
		}
		if containsAny(job, "kube-state", "kube_state", "ksm") {
			// kube-state-metrics 的资源类型由具体标签决定，这里默认 pod。
			return "pod"
		}
	}
	// 通过 instance 推断：如果 instance 是 IP:Port 且没有 pod 标签，可能是 node 级指标。
	if string(metric["instance"]) != "" && string(metric["pod"]) == "" {
		return "node"
	}
	return "unknown"
}

// containsAny 检查字符串是否包含任一子串。
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
