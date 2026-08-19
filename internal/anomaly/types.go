package anomaly

import "time"

// DataPoint 是一个时间序列数据点。
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// Severity 异常级别。
const (
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Anomaly 是检测到的异常点。
type Anomaly struct {
	Metric       string    `json:"metric"`
	Timestamp    time.Time `json:"timestamp"`
	Value        float64   `json:"value"`
	AnomalyScore float64   `json:"anomaly_score"` // 0.0 ~ 1.0，越大越异常
	Severity     string    `json:"severity"`      // warning / critical
	Reason       string    `json:"reason"`
	Detector     string    `json:"detector"`
}

// Detector 是异常检测器接口。
// 所有检测算法（静态阈值、移动平均、EWMA、Z-Score 等）都实现此接口。
type Detector interface {
	// Name 返回检测器名称。
	Name() string
	// Detect 对时间序列进行异常检测，返回所有异常点。
	Detect(metric string, points []DataPoint) []Anomaly
}

// Engine 是异常检测引擎，组合多个检测器。
type Engine struct {
	detectors []Detector
}

// NewEngine 创建异常检测引擎。
func NewEngine(detectors ...Detector) *Engine {
	return &Engine{detectors: detectors}
}

// AddDetector 添加检测器。
func (e *Engine) AddDetector(d Detector) {
	e.detectors = append(e.detectors, d)
}

// Detect 运行所有检测器，合并结果。
func (e *Engine) Detect(metric string, points []DataPoint) []Anomaly {
	var result []Anomaly
	for _, d := range e.detectors {
		result = append(result, d.Detect(metric, points)...)
	}
	return result
}
