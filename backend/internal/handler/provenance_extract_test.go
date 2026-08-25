package handler

import (
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/prometheus/common/model"
)

func TestExtractLatestSampleTimestamp_Vector(t *testing.T) {
	ts1 := model.Time(time.Date(2026, 8, 24, 18, 15, 0, 0, time.UTC).UnixNano() / int64(time.Millisecond))
	ts2 := model.Time(time.Date(2026, 8, 24, 18, 15, 30, 0, time.UTC).UnixNano() / int64(time.Millisecond))

	vector := model.Vector{
		{Metric: model.Metric{"__name__": "up"}, Timestamp: ts1, Value: 1},
		{Metric: model.Metric{"__name__": "up"}, Timestamp: ts2, Value: 1},
	}

	result := &monitoring.QueryResult{
		ResultType: "vector",
		Result:     vector,
	}

	dataTS, ok := extractLatestSampleTimestamp(result)
	if !ok {
		t.Fatal("should find timestamp")
	}
	expected := time.Unix(0, int64(ts2)*int64(time.Millisecond)).UTC()
	if !dataTS.Equal(expected) {
		t.Errorf("dataTS = %v, want %v", dataTS, expected)
	}
}

func TestExtractLatestSampleTimestamp_Empty(t *testing.T) {
	result := &monitoring.QueryResult{
		ResultType: "vector",
		Result:     model.Vector{},
	}

	_, ok := extractLatestSampleTimestamp(result)
	if ok {
		t.Error("should not find timestamp in empty vector")
	}
}

func TestExtractLatestSampleTimestamp_NilResult(t *testing.T) {
	_, ok := extractLatestSampleTimestamp(nil)
	if ok {
		t.Error("should not find timestamp in nil result")
	}

	result := &monitoring.QueryResult{Result: nil}
	_, ok = extractLatestSampleTimestamp(result)
	if ok {
		t.Error("should not find timestamp in nil Result")
	}
}

func TestExtractLatestSampleTimestamp_Matrix(t *testing.T) {
	ts1 := model.Time(time.Date(2026, 8, 24, 18, 15, 0, 0, time.UTC).UnixNano() / int64(time.Millisecond))
	ts2 := model.Time(time.Date(2026, 8, 24, 18, 15, 15, 0, time.UTC).UnixNano() / int64(time.Millisecond))
	ts3 := model.Time(time.Date(2026, 8, 24, 18, 15, 30, 0, time.UTC).UnixNano() / int64(time.Millisecond))

	matrix := model.Matrix{
		{
			Metric: model.Metric{"__name__": "cpu"},
			Values: []model.SamplePair{
				{Timestamp: ts1, Value: 50},
				{Timestamp: ts2, Value: 55},
			},
		},
		{
			Metric: model.Metric{"__name__": "mem"},
			Values: []model.SamplePair{
				{Timestamp: ts3, Value: 70},
			},
		},
	}

	result := &monitoring.QueryResult{
		ResultType: "matrix",
		Result:     matrix,
	}

	dataTS, ok := extractLatestSampleTimestamp(result)
	if !ok {
		t.Fatal("should find timestamp")
	}
	expected := time.Unix(0, int64(ts3)*int64(time.Millisecond)).UTC()
	if !dataTS.Equal(expected) {
		t.Errorf("dataTS = %v, want %v (max timestamp)", dataTS, expected)
	}
}
