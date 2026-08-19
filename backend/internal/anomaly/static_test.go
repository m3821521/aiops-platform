package anomaly_test

import (
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
)

func TestStaticThresholdUpperWarning(t *testing.T) {
	warning := 80.0
	detector := anomaly.NewStaticThresholdDetector(&warning, nil, nil, nil)

	points := []anomaly.DataPoint{
		{Timestamp: time.Now(), Value: 50}, // 正常
		{Timestamp: time.Now(), Value: 85}, // warning
	}

	results := detector.Detect("cpu", points)
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].Severity != anomaly.SeverityWarning {
		t.Fatalf("expected warning, got %s", results[0].Severity)
	}
	if results[0].Value != 85 {
		t.Fatalf("expected value 85, got %f", results[0].Value)
	}
}

func TestStaticThresholdUpperCritical(t *testing.T) {
	warning := 80.0
	critical := 90.0
	detector := anomaly.NewStaticThresholdDetector(&warning, &critical, nil, nil)

	points := []anomaly.DataPoint{
		{Timestamp: time.Now(), Value: 95}, // critical
	}

	results := detector.Detect("cpu", points)
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].Severity != anomaly.SeverityCritical {
		t.Fatalf("expected critical, got %s", results[0].Severity)
	}
	if results[0].AnomalyScore != 1.0 {
		t.Fatalf("expected score 1.0, got %f", results[0].AnomalyScore)
	}
}

func TestStaticThresholdLower(t *testing.T) {
	warning := 20.0
	detector := anomaly.NewStaticThresholdDetector(nil, nil, &warning, nil)

	points := []anomaly.DataPoint{
		{Timestamp: time.Now(), Value: 50}, // 正常
		{Timestamp: time.Now(), Value: 10}, // 低于 warning
	}

	results := detector.Detect("memory", points)
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].Value != 10 {
		t.Fatalf("expected value 10, got %f", results[0].Value)
	}
}

func TestStaticThresholdNoAnomaly(t *testing.T) {
	warning := 80.0
	detector := anomaly.NewStaticThresholdDetector(&warning, nil, nil, nil)

	points := []anomaly.DataPoint{
		{Timestamp: time.Now(), Value: 50},
		{Timestamp: time.Now(), Value: 70},
	}

	results := detector.Detect("cpu", points)
	if len(results) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(results))
	}
}

func TestStaticThresholdScoreInterpolation(t *testing.T) {
	warning := 80.0
	critical := 90.0
	detector := anomaly.NewStaticThresholdDetector(&warning, &critical, nil, nil)

	// 85 在 warning 和 critical 中间，score 应该在 0.5~1.0 之间。
	points := []anomaly.DataPoint{
		{Timestamp: time.Now(), Value: 85},
	}

	results := detector.Detect("cpu", points)
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].AnomalyScore <= 0.5 || results[0].AnomalyScore >= 1.0 {
		t.Fatalf("expected score between 0.5 and 1.0, got %f", results[0].AnomalyScore)
	}
}

func TestEngineMultipleDetectors(t *testing.T) {
	warning := 80.0
	d1 := anomaly.NewStaticThresholdDetector(&warning, nil, nil, nil)
	lowerWarn := 20.0
	d2 := anomaly.NewStaticThresholdDetector(nil, nil, &lowerWarn, nil)

	engine := anomaly.NewEngine(d1, d2)

	points := []anomaly.DataPoint{
		{Timestamp: time.Now(), Value: 85}, // d1 检测到
		{Timestamp: time.Now(), Value: 10}, // d2 检测到
	}

	results := engine.Detect("cpu", points)
	if len(results) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(results))
	}
}
