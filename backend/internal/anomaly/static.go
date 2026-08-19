package anomaly

import (
	"fmt"
	"math"
)

// StaticThresholdDetector 静态阈值检测器。
// 超过 UpperWarning / UpperCritical 或低于 LowerWarning / LowerCritical 时判定为异常。
type StaticThresholdDetector struct {
	UpperWarning  *float64 // 上限 warning 阈值
	UpperCritical *float64 // 上限 critical 阈值
	LowerWarning  *float64 // 下限 warning 阈值
	LowerCritical *float64 // 下限 critical 阈值
}

// NewStaticThresholdDetector 创建静态阈值检测器。
// 任一阈值传 nil 表示不检测该方向。
func NewStaticThresholdDetector(upperWarning, upperCritical, lowerWarning, lowerCritical *float64) *StaticThresholdDetector {
	return &StaticThresholdDetector{
		UpperWarning:  upperWarning,
		UpperCritical: upperCritical,
		LowerWarning:  lowerWarning,
		LowerCritical: lowerCritical,
	}
}

// Name 实现 Detector 接口。
func (d *StaticThresholdDetector) Name() string {
	return "static_threshold"
}

// Detect 实现 Detector 接口。
func (d *StaticThresholdDetector) Detect(metric string, points []DataPoint) []Anomaly {
	var anomalies []Anomaly
	for _, p := range points {
		if a, ok := d.checkPoint(metric, p); ok {
			anomalies = append(anomalies, a)
		}
	}
	return anomalies
}

// checkPoint 检查单个数据点是否异常。
func (d *StaticThresholdDetector) checkPoint(metric string, p DataPoint) (Anomaly, bool) {
	// 上限检测。
	if d.UpperCritical != nil && p.Value >= *d.UpperCritical {
		return Anomaly{
			Metric:       metric,
			Timestamp:    p.Timestamp,
			Value:        p.Value,
			AnomalyScore: 1.0,
			Severity:     SeverityCritical,
			Reason:       fmt.Sprintf("值 %.2f 超过 critical 阈值 %.2f", p.Value, *d.UpperCritical),
			Detector:     d.Name(),
		}, true
	}
	if d.UpperWarning != nil && p.Value >= *d.UpperWarning {
		score := 0.5
		if d.UpperCritical != nil && *d.UpperCritical > *d.UpperWarning {
			// 在 warning 和 critical 之间线性插值。
			ratio := (p.Value - *d.UpperWarning) / (*d.UpperCritical - *d.UpperWarning)
			score = 0.5 + 0.5*math.Max(0, math.Min(1, ratio))
		}
		return Anomaly{
			Metric:       metric,
			Timestamp:    p.Timestamp,
			Value:        p.Value,
			AnomalyScore: score,
			Severity:     SeverityWarning,
			Reason:       fmt.Sprintf("值 %.2f 超过 warning 阈值 %.2f", p.Value, *d.UpperWarning),
			Detector:     d.Name(),
		}, true
	}

	// 下限检测。
	if d.LowerCritical != nil && p.Value <= *d.LowerCritical {
		return Anomaly{
			Metric:       metric,
			Timestamp:    p.Timestamp,
			Value:        p.Value,
			AnomalyScore: 1.0,
			Severity:     SeverityCritical,
			Reason:       fmt.Sprintf("值 %.2f 低于 critical 阈值 %.2f", p.Value, *d.LowerCritical),
			Detector:     d.Name(),
		}, true
	}
	if d.LowerWarning != nil && p.Value <= *d.LowerWarning {
		score := 0.5
		if d.LowerCritical != nil && *d.LowerWarning > *d.LowerCritical {
			ratio := (*d.LowerWarning - p.Value) / (*d.LowerWarning - *d.LowerCritical)
			score = 0.5 + 0.5*math.Max(0, math.Min(1, ratio))
		}
		return Anomaly{
			Metric:       metric,
			Timestamp:    p.Timestamp,
			Value:        p.Value,
			AnomalyScore: score,
			Severity:     SeverityWarning,
			Reason:       fmt.Sprintf("值 %.2f 低于 warning 阈值 %.2f", p.Value, *d.LowerWarning),
			Detector:     d.Name(),
		}, true
	}

	return Anomaly{}, false
}

// floatPtr 辅助函数，用于创建 *float64。
func floatPtr(v float64) *float64 {
	return &v
}
