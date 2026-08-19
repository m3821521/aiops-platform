package anomaly

import (
	"fmt"
	"math"
)

// MovingAverageDetector 简单移动平均检测器。
// 当前值偏离前 N 个点的均值超过指定比例时，判定为异常。
type MovingAverageDetector struct {
	Window    int     // 窗口大小（数据点数）
	Threshold float64 // 偏离比例阈值，如 0.2 表示偏离 20%
}

// NewMovingAverageDetector 创建移动平均检测器。
func NewMovingAverageDetector(window int, threshold float64) *MovingAverageDetector {
	if window < 1 {
		window = 5
	}
	if threshold <= 0 {
		threshold = 0.2
	}
	return &MovingAverageDetector{Window: window, Threshold: threshold}
}

func (d *MovingAverageDetector) Name() string {
	return "moving_average"
}

func (d *MovingAverageDetector) Detect(metric string, points []DataPoint) []Anomaly {
	var anomalies []Anomaly
	for i := d.Window; i < len(points); i++ {
		// 计算前 N 个点的均值。
		sum := 0.0
		for j := i - d.Window; j < i; j++ {
			sum += points[j].Value
		}
		avg := sum / float64(d.Window)
		if avg == 0 {
			continue
		}

		deviation := math.Abs(points[i].Value-avg) / math.Abs(avg)
		if deviation >= d.Threshold {
			severity := SeverityWarning
			score := math.Min(1.0, deviation/d.Threshold)
			if deviation >= d.Threshold*2 {
				severity = SeverityCritical
				score = 1.0
			}
			anomalies = append(anomalies, Anomaly{
				Metric:       metric,
				Timestamp:    points[i].Timestamp,
				Value:        points[i].Value,
				AnomalyScore: score,
				Severity:     severity,
				Reason:       fmt.Sprintf("值 %.2f 偏离移动平均 %.2f（偏离 %.1f%%）", points[i].Value, avg, deviation*100),
				Detector:     d.Name(),
			})
		}
	}
	return anomalies
}

// EWMADetector 指数加权移动平均检测器。
type EWMADetector struct {
	Alpha     float64 // 平滑系数，0-1，越大越重视近期数据
	Threshold float64 // 偏离比例阈值
}

// NewEWMADetector 创建 EWMA 检测器。
func NewEWMADetector(alpha, threshold float64) *EWMADetector {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3
	}
	if threshold <= 0 {
		threshold = 0.2
	}
	return &EWMADetector{Alpha: alpha, Threshold: threshold}
}

func (d *EWMADetector) Name() string {
	return "ewma"
}

func (d *EWMADetector) Detect(metric string, points []DataPoint) []Anomaly {
	var anomalies []Anomaly
	if len(points) < 2 {
		return anomalies
	}

	ewma := points[0].Value
	for i := 1; i < len(points); i++ {
		// EWMA 更新。
		ewma = d.Alpha*points[i].Value + (1-d.Alpha)*ewma

		if ewma == 0 {
			continue
		}
		deviation := math.Abs(points[i].Value-ewma) / math.Abs(ewma)
		if deviation >= d.Threshold {
			severity := SeverityWarning
			score := math.Min(1.0, deviation/d.Threshold)
			if deviation >= d.Threshold*2 {
				severity = SeverityCritical
				score = 1.0
			}
			anomalies = append(anomalies, Anomaly{
				Metric:       metric,
				Timestamp:    points[i].Timestamp,
				Value:        points[i].Value,
				AnomalyScore: score,
				Severity:     severity,
				Reason:       fmt.Sprintf("值 %.2f 偏离 EWMA %.2f（偏离 %.1f%%）", points[i].Value, ewma, deviation*100),
				Detector:     d.Name(),
			})
		}
	}
	return anomalies
}

// ZScoreDetector Z-Score 检测器。
// 当前值偏离前 N 个点均值超过指定标准差倍数时，判定为异常。
type ZScoreDetector struct {
	Window    int     // 窗口大小
	Threshold float64 // Z-Score 阈值，如 2.0 表示 2 个标准差
}

// NewZScoreDetector 创建 Z-Score 检测器。
func NewZScoreDetector(window int, threshold float64) *ZScoreDetector {
	if window < 2 {
		window = 5
	}
	if threshold <= 0 {
		threshold = 2.0
	}
	return &ZScoreDetector{Window: window, Threshold: threshold}
}

func (d *ZScoreDetector) Name() string {
	return "z_score"
}

func (d *ZScoreDetector) Detect(metric string, points []DataPoint) []Anomaly {
	var anomalies []Anomaly
	for i := d.Window; i < len(points); i++ {
		// 计算前 N 个点的均值和标准差。
		window := points[i-d.Window : i]
		mean, stddev := meanAndStddev(window)
		if stddev == 0 {
			// 标准差为 0 但当前值偏离均值，视为突变异常。
			if points[i].Value != mean {
				anomalies = append(anomalies, Anomaly{
					Metric:       metric,
					Timestamp:    points[i].Timestamp,
					Value:        points[i].Value,
					AnomalyScore: 1.0,
					Severity:     SeverityCritical,
					Reason:       fmt.Sprintf("值 %.2f 偏离恒定基线 %.2f", points[i].Value, mean),
					Detector:     d.Name(),
				})
			}
			continue
		}

		zScore := (points[i].Value - mean) / stddev
		absZ := math.Abs(zScore)
		if absZ >= d.Threshold {
			severity := SeverityWarning
			score := math.Min(1.0, absZ/(d.Threshold*2))
			if absZ >= d.Threshold*1.5 {
				severity = SeverityCritical
				score = 1.0
			}
			anomalies = append(anomalies, Anomaly{
				Metric:       metric,
				Timestamp:    points[i].Timestamp,
				Value:        points[i].Value,
				AnomalyScore: score,
				Severity:     severity,
				Reason:       fmt.Sprintf("Z-Score=%.2f（均值=%.2f, 标准差=%.2f）", zScore, mean, stddev),
				Detector:     d.Name(),
			})
		}
	}
	return anomalies
}

// meanAndStddev 计算一组数据点的均值和总体标准差。
func meanAndStddev(points []DataPoint) (float64, float64) {
	if len(points) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, p := range points {
		sum += p.Value
	}
	mean := sum / float64(len(points))

	variance := 0.0
	for _, p := range points {
		diff := p.Value - mean
		variance += diff * diff
	}
	variance /= float64(len(points))
	return mean, math.Sqrt(variance)
}
