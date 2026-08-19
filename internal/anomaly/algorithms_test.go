package anomaly_test

import (
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
)

func genPoints(values []float64) []anomaly.DataPoint {
	points := make([]anomaly.DataPoint, len(values))
	now := time.Now()
	for i, v := range values {
		points[i] = anomaly.DataPoint{Timestamp: now.Add(time.Duration(i) * time.Minute), Value: v}
	}
	return points
}

func TestMovingAverageDetect(t *testing.T) {
	// 前 5 个点稳定在 50，第 6 个点突增到 100。
	points := genPoints([]float64{50, 51, 49, 50, 52, 100})
	detector := anomaly.NewMovingAverageDetector(5, 0.2)

	results := detector.Detect("cpu", points)
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].Value != 100 {
		t.Fatalf("expected value 100, got %f", results[0].Value)
	}
}

func TestMovingAverageNoAnomaly(t *testing.T) {
	points := genPoints([]float64{50, 51, 49, 50, 52, 51})
	detector := anomaly.NewMovingAverageDetector(5, 0.2)

	results := detector.Detect("cpu", points)
	if len(results) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(results))
	}
}

func TestEWMADetect(t *testing.T) {
	// 稳定在 50，然后突增。
	points := genPoints([]float64{50, 50, 50, 50, 50, 90})
	detector := anomaly.NewEWMADetector(0.3, 0.2)

	results := detector.Detect("cpu", points)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 anomaly, got %d", len(results))
	}
}

func TestEWMAShortSeries(t *testing.T) {
	// 少于 2 个点，不应有异常。
	points := genPoints([]float64{50})
	detector := anomaly.NewEWMADetector(0.3, 0.2)

	results := detector.Detect("cpu", points)
	if len(results) != 0 {
		t.Fatalf("expected 0 anomalies for short series, got %d", len(results))
	}
}

func TestZScoreDetect(t *testing.T) {
	// 前 5 个点稳定在 50，第 6 个点突增到 100。
	points := genPoints([]float64{50, 50, 50, 50, 50, 100})
	detector := anomaly.NewZScoreDetector(5, 2.0)

	results := detector.Detect("cpu", points)
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].Value != 100 {
		t.Fatalf("expected value 100, got %f", results[0].Value)
	}
}

func TestZScoreNoAnomaly(t *testing.T) {
	// 数据波动小，Z-Score 不超过阈值。
	points := genPoints([]float64{50, 51, 49, 50, 52, 51})
	detector := anomaly.NewZScoreDetector(5, 2.0)

	results := detector.Detect("cpu", points)
	if len(results) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(results))
	}
}

func TestZScoreZeroStddev(t *testing.T) {
	// 所有值相同，标准差为 0，不应 panic。
	points := genPoints([]float64{50, 50, 50, 50, 50, 50})
	detector := anomaly.NewZScoreDetector(5, 2.0)

	results := detector.Detect("cpu", points)
	if len(results) != 0 {
		t.Fatalf("expected 0 anomalies for zero stddev, got %d", len(results))
	}
}

func TestEngineWithAllDetectors(t *testing.T) {
	// 组合所有检测器。
	engine := anomaly.NewEngine(
		anomaly.NewMovingAverageDetector(5, 0.2),
		anomaly.NewEWMADetector(0.3, 0.2),
		anomaly.NewZScoreDetector(5, 2.0),
	)

	points := genPoints([]float64{50, 50, 50, 50, 50, 100})
	results := engine.Detect("cpu", points)

	// 三个检测器都应该检测到这个突增。
	if len(results) < 3 {
		t.Fatalf("expected at least 3 anomalies (one per detector), got %d", len(results))
	}
}
