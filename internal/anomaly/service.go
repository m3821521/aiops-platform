package anomaly

import (
	"context"
	"fmt"
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
	Query      string          `json:"query"`
	Start      time.Time       `json:"start"`
	End        time.Time       `json:"end"`
	Step       time.Duration   `json:"step"`
	Thresholds ThresholdConfig `json:"thresholds"`
}

// DetectResult 异常检测结果。
type DetectResult struct {
	Metric        string    `json:"metric"`
	PointsChecked int       `json:"points_checked"`
	Anomalies     []Anomaly `json:"anomalies"`
}

// Service 异常检测服务，从 Prometheus 拉取数据并运行检测器。
type Service struct {
	querier monitoring.Querier
}

// NewService 创建异常检测服务。
func NewService(querier monitoring.Querier) *Service {
	return &Service{querier: querier}
}

// Detect 执行异常检测。
// 1. 从 Prometheus 查询范围数据
// 2. 转换为 DataPoint 序列
// 3. 用静态阈值检测器检测
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

	// 从 Prometheus 查询范围数据。
	result, err := s.querier.QueryRange(ctx, req.Query, req.Start, req.End, req.Step)
	if err != nil {
		return nil, fmt.Errorf("查询 Prometheus 失败: %w", err)
	}

	// 转换为 DataPoint。
	points, metricName, err := matrixToPoints(result)
	if err != nil {
		return nil, err
	}

	// 构建静态阈值检测器。
	detector := NewStaticThresholdDetector(
		req.Thresholds.UpperWarning,
		req.Thresholds.UpperCritical,
		req.Thresholds.LowerWarning,
		req.Thresholds.LowerCritical,
	)

	// 检测。
	anomalies := detector.Detect(metricName, points)

	return &DetectResult{
		Metric:        metricName,
		PointsChecked: len(points),
		Anomalies:     anomalies,
	}, nil
}

// matrixToPoints 将 Prometheus 查询结果转换为 DataPoint 切片。
// 取第一条时间序列（单指标检测）。
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
