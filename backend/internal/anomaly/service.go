package anomaly

import (
	"context"
	"fmt"
	"log/slog"
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
		resourceType := req.ResourceType
		if resourceType == "" {
			resourceType = "pod"
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

		// 持久化每个异常点。
		for _, a := range anomalies {
			rec := &AnomalyRecord{
				Metric:       metricName,
				ResourceType: resourceType,
				ResourceName: resourceName,
				Namespace:    namespace,
				Cluster:      cluster,
				Timestamp:    a.Timestamp,
				Value:        a.Value,
				AnomalyScore: a.AnomalyScore,
				Severity:     a.Severity,
				Algorithm:    a.Detector,
				Reason:       a.Reason,
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

			if s.repo != nil {
				saved, isNew, err := s.repo.Upsert(ctx, rec)
				if err != nil {
					slog.Warn("anomaly: persist failed", "metric", metricName, "err", err)
					continue
				}
				if isNew {
					savedCount++
				}
				rec = saved

				// 严重异常进入 Incident。
				if s.incidentSink != nil && (a.Severity == SeverityCritical || a.Severity == SeverityWarning) {
					incidentID, err := s.incidentSink.IngestAnomalySignal(ctx, AnomalySignal{
						ID:           rec.ID,
						Title:        fmt.Sprintf("%s: %s", a.Severity, metricName),
						Severity:     a.Severity,
						Cluster:      cluster,
						Namespace:    namespace,
						ResourceType: resourceType,
						ResourceName: resourceName,
						Timestamp:    a.Timestamp,
						Metric:       metricName,
						Value:        a.Value,
						AnomalyScore: a.AnomalyScore,
						Reason:       a.Reason,
						Algorithm:    a.Detector,
					})
					if err == nil && incidentID > 0 {
						_ = s.repo.UpdateIncident(ctx, rec.ID, incidentID)
					}
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
