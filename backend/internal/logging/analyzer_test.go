package logging_test

import (
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/logging"
)

func TestNormalizeMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Connection refused to 192.168.1.100:8080",
			expected: "Connection refused to <ip>",
		},
		{
			input:    "User 12345 failed to login",
			expected: "User <num> failed to login",
		},
		{
			input:    "Request id=550e8400-e29b-41d4-a716-446655440000 failed",
			expected: "Request id=<uuid> failed",
		},
		{
			input:    `Error: "database connection lost"`,
			expected: "Error: <str>",
		},
		{
			input:    "Memory address 0x7fff5fbff8c0 invalid",
			expected: "Memory address <hex> invalid",
		},
	}

	for _, tt := range tests {
		result := logging.NormalizeMessage(tt.input)
		if result != tt.expected {
			t.Errorf("normalize(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestAnalyzerAggregate(t *testing.T) {
	now := time.Now()
	hits := []logging.LogHit{
		{Message: "Connection refused to 10.0.0.1:8080", Timestamp: now, Pod: "svc-a-1", Namespace: "default"},
		{Message: "Connection refused to 10.0.0.2:8080", Timestamp: now.Add(time.Minute), Pod: "svc-a-2", Namespace: "default"},
		{Message: "Connection refused to 10.0.0.3:9090", Timestamp: now.Add(2 * time.Minute), Pod: "svc-b-1", Namespace: "default"},
		{Message: "Timeout after 30s", Timestamp: now.Add(3 * time.Minute), Pod: "svc-c-1", Namespace: "kube-system"},
	}

	analyzer := logging.NewAnalyzer(2, 1*time.Hour)
	result := analyzer.Analyze(hits)

	if result.TotalLogs != 4 {
		t.Fatalf("expected 4 total logs, got %d", result.TotalLogs)
	}
	// 应该有 2 组：Connection refused（3条）和 Timeout（1条）。
	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
	// 第一组应该是 Connection refused（count=3）。
	if result.Groups[0].Count != 3 {
		t.Fatalf("expected first group count 3, got %d", result.Groups[0].Count)
	}
	// Connection refused 出现 3 次 >= 阈值 2，应该是高频。
	if !result.Groups[0].IsHighFrequency {
		t.Fatal("expected first group to be high frequency")
	}
	// 受影响服务应该有 3 个（svc-a-1, svc-a-2, svc-b-1）。
	if len(result.Groups[0].Services) != 3 {
		t.Fatalf("expected 3 services in first group, got %d", len(result.Groups[0].Services))
	}
	// 第二组 Timeout 只有 1 次，不是高频。
	if result.Groups[1].IsHighFrequency {
		t.Fatal("expected second group to NOT be high frequency")
	}
}

func TestAnalyzerEmpty(t *testing.T) {
	analyzer := logging.NewAnalyzer(10, 1*time.Hour)
	result := analyzer.Analyze(nil)

	if result.TotalLogs != 0 {
		t.Fatalf("expected 0 total logs, got %d", result.TotalLogs)
	}
	if len(result.Groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(result.Groups))
	}
}

func TestAnalyzerNewAnomaly(t *testing.T) {
	now := time.Now()
	// 新异常：首次出现在最近 1 小时内。
	hits := []logging.LogHit{
		{Message: "New error type XYZ", Timestamp: now, Pod: "svc-a"},
	}

	analyzer := logging.NewAnalyzer(10, 1*time.Hour)
	result := analyzer.Analyze(hits)

	if result.NewCount != 1 {
		t.Fatalf("expected 1 new anomaly, got %d", result.NewCount)
	}
	if !result.Groups[0].IsNew {
		t.Fatal("expected group to be new")
	}
}

func TestAnalyzerOldAnomaly(t *testing.T) {
	// 旧异常：首次出现在 2 小时前。
	old := time.Now().Add(-2 * time.Hour)
	hits := []logging.LogHit{
		{Message: "Old error type", Timestamp: old, Pod: "svc-a"},
	}

	analyzer := logging.NewAnalyzer(10, 1*time.Hour)
	result := analyzer.Analyze(hits)

	if result.NewCount != 0 {
		t.Fatalf("expected 0 new anomalies, got %d", result.NewCount)
	}
	if result.Groups[0].IsNew {
		t.Fatal("expected group to NOT be new")
	}
}

func TestAnalyzerSortByCount(t *testing.T) {
	now := time.Now()
	hits := []logging.LogHit{
		{Message: "Rare error", Timestamp: now, Pod: "svc-a"},
		{Message: "Frequent error 1", Timestamp: now, Pod: "svc-a"},
		{Message: "Frequent error 2", Timestamp: now, Pod: "svc-a"},
		{Message: "Frequent error 3", Timestamp: now, Pod: "svc-a"},
	}

	analyzer := logging.NewAnalyzer(10, 1*time.Hour)
	result := analyzer.Analyze(hits)

	// 三组：Frequent error（3条，归一化后相同）、Rare error（1条）。
	// 等等，"Frequent error 1/2/3" 归一化后数字会变成 <num>，所以是同一个模板。
	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
	// 第一组 count 应该 >= 第二组。
	if result.Groups[0].Count < result.Groups[1].Count {
		t.Fatal("groups should be sorted by count descending")
	}
}

func TestAnalyzerSampleMessage(t *testing.T) {
	now := time.Now()
	hits := []logging.LogHit{
		{Message: "Connection refused to 10.0.0.1:8080", Timestamp: now, Pod: "svc-a"},
		{Message: "Connection refused to 10.0.0.2:8080", Timestamp: now.Add(time.Minute), Pod: "svc-b"},
	}

	analyzer := logging.NewAnalyzer(10, 1*time.Hour)
	result := analyzer.Analyze(hits)

	if result.Groups[0].SampleMessage == "" {
		t.Fatal("expected sample message to be set")
	}
	// 示例消息应该是原始消息，不是归一化后的。
	if result.Groups[0].SampleMessage == result.Groups[0].Template {
		t.Fatal("sample message should be original, not template")
	}
}
