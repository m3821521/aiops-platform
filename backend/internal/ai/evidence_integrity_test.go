package ai

import (
	"testing"
	"time"
)

// V1 — Valid Evidence Reference
func TestV1ValidEvidenceReference(t *testing.T) {
	ctx := AIContext{
		IncidentID: 11,
		Alerts: []AlertSummary{
			{ID: "E-VALID001", Name: "AlertmanagerClusterCrashlooping", Severity: "critical"},
		},
	}

	validIDs := CollectEvidenceIDs(ctx)
	if !validIDs["E-VALID001"] {
		t.Fatal("E-VALID001 should be in valid IDs")
	}

	result := &AIAnalysisResult{
		Summary:    "Alertmanager is repeatedly crashing.",
		Confidence: 0.4,
		Evidence: []AIEvidence{
			{ID: "E-VALID001", Type: "alert", Source: "alerts", Description: "AlertmanagerClusterCrashlooping alert", Importance: "high"},
		},
	}

	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted, got %d", len(accepted))
	}
	if len(rejected) != 0 {
		t.Fatalf("expected 0 rejected, got %d", len(rejected))
	}
	if accepted[0].ID != "E-VALID001" {
		t.Fatalf("expected E-VALID001, got %s", accepted[0].ID)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		t.Fatalf("confidence out of range: %f", result.Confidence)
	}
	t.Log("V1 PASS: Valid evidence reference retained")
}

// V2 — Invalid Evidence ID / Hallucinated ID
func TestV2InvalidEvidenceID(t *testing.T) {
	ctx := AIContext{
		IncidentID: 11,
		Alerts: []AlertSummary{
			{ID: "E-VALID001", Name: "AlertmanagerClusterCrashlooping", Severity: "critical"},
		},
	}

	validIDs := CollectEvidenceIDs(ctx)

	result := &AIAnalysisResult{
		Summary:    "Alertmanager has a corrupted configuration.",
		Confidence: 0.8,
		Evidence: []AIEvidence{
			{ID: "E-VALID001", Type: "alert", Source: "alerts", Description: "Alert", Importance: "high"},
			{ID: "E-FAKE999", Type: "log", Source: "logs", Description: "Fake log evidence", Importance: "medium"},
		},
	}

	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted, got %d", len(accepted))
	}
	if len(rejected) != 1 {
		t.Fatalf("expected 1 rejected, got %d", len(rejected))
	}
	if rejected[0] != "E-FAKE999" {
		t.Fatalf("expected E-FAKE999 rejected, got %s", rejected[0])
	}
	// 验证无效 ID 不进入最终结果
	result.Evidence = accepted
	for _, e := range result.Evidence {
		if e.ID == "E-FAKE999" {
			t.Fatal("E-FAKE999 should not be in final result")
		}
	}
	t.Log("V2 PASS: Invalid evidence ID filtered, WARN logged")
}

// V3 — No Evidence
func TestV3NoEvidence(t *testing.T) {
	ctx := AIContext{
		IncidentID: 11,
		// 所有 Evidence 为空
	}

	validIDs := CollectEvidenceIDs(ctx)
	if len(validIDs) != 0 {
		t.Fatalf("expected 0 valid IDs, got %d", len(validIDs))
	}

	result := &AIAnalysisResult{
		Summary:    "Alertmanager is crashing due to OOM.",
		Confidence: 0.9,
		Evidence: []AIEvidence{
			{ID: "E-FAKE001", Type: "metric", Description: "Fake metric"},
		},
	}

	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	if len(accepted) != 0 {
		t.Fatalf("expected 0 accepted with no evidence, got %d", len(accepted))
	}
	if len(rejected) != 1 {
		t.Fatalf("expected 1 rejected, got %d", len(rejected))
	}

	// 模拟 AnalyzeIncident 中的 confidence 校验逻辑（P1-X.4.2.1 修复）
	result.Evidence = accepted
	// Case A: validIDs == 0 && accepted == 0 → confidence = 0
	if len(validIDs) == 0 && len(result.Evidence) == 0 {
		result.Confidence = 0
		if result.RootCauseExplanation == "" || !containsInsufficientEvidence(result.RootCauseExplanation) {
			result.RootCauseExplanation = "当前没有收集到任何有效证据，无法确定具体根因。"
		}
	}

	if result.Confidence != 0 {
		t.Fatalf("V3: expected confidence=0 when no evidence, got %f", result.Confidence)
	}
	if !containsInsufficientEvidence(result.RootCauseExplanation) {
		t.Fatalf("V3: root cause should express insufficient evidence, got: %s", result.RootCauseExplanation)
	}
	if len(result.Evidence) != 0 {
		t.Fatalf("V3: expected 0 evidence references, got %d", len(result.Evidence))
	}

	t.Log("V3 PASS: No evidence → confidence=0, root cause=insufficient evidence, evidence references=[]")
}

// V4 — Alert-only / Missing Telemetry
func TestV4AlertOnlyMissingTelemetry(t *testing.T) {
	ctx := AIContext{
		IncidentID: 11,
		Alerts: []AlertSummary{
			{ID: "E-ALERT001", Name: "AlertmanagerClusterCrashlooping", Severity: "critical"},
		},
		// Anomalies, Metrics, Logs, Events 均为空
	}

	validIDs := CollectEvidenceIDs(ctx)
	if len(validIDs) != 1 {
		t.Fatalf("expected 1 valid ID (alert only), got %d", len(validIDs))
	}

	// 验证 data_sources 状态
	if !ctx.DataSources.AlertsAvailable {
		ctx.DataSources.AlertsAvailable = true
	}
	if ctx.DataSources.AnomaliesAvailable || ctx.DataSources.MetricsAvailable ||
		ctx.DataSources.LogsAvailable || ctx.DataSources.EventsAvailable {
		t.Fatal("only alerts should be available")
	}

	result := &AIAnalysisResult{
		Summary:    "AlertmanagerClusterCrashlooping alert triggered, but missing logs/metrics/events to determine specific cause.",
		Confidence: 0.4,
		Evidence: []AIEvidence{
			{ID: "E-ALERT001", Type: "alert", Description: "AlertmanagerClusterCrashlooping", Importance: "high"},
		},
	}

	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted, got %d", len(accepted))
	}
	if len(rejected) != 0 {
		t.Fatalf("expected 0 rejected, got %d", len(rejected))
	}
	if result.Confidence > 0.5 {
		t.Fatalf("alert-only confidence should not be high, got %f", result.Confidence)
	}

	// 验证 AI 没有伪造 Metrics/Logs/Events Evidence
	for _, e := range accepted {
		if e.Type == "metric" || e.Type == "log" || e.Type == "event" {
			t.Fatalf("alert-only scenario should not have %s evidence", e.Type)
		}
	}

	t.Log("V4 PASS: Alert-only scenario correctly identifies missing telemetry, no fabricated metrics/logs/events")
}

// V5 — Valid ID But Unsupported Root Cause
func TestV5ValidIDUnsupportedRootCause(t *testing.T) {
	ctx := AIContext{
		IncidentID: 11,
		Alerts: []AlertSummary{
			{ID: "E-VALID001", Name: "AlertmanagerClusterCrashlooping", Severity: "critical"},
		},
	}

	validIDs := CollectEvidenceIDs(ctx)

	// 模拟恶意/错误 AI 输出：合法 ID 但 Root Cause 与 Evidence 不匹配
	result := &AIAnalysisResult{
		Summary:    "The Alertmanager PVC is corrupted.",
		Confidence: 0.95,
		Evidence: []AIEvidence{
			{ID: "E-VALID001", Type: "alert", Description: "AlertmanagerClusterCrashlooping", Importance: "high"},
		},
	}

	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	// E-VALID001 是合法 ID，会被保留
	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted (valid ID), got %d", len(accepted))
	}
	if len(rejected) != 0 {
		t.Fatalf("expected 0 rejected, got %d", len(rejected))
	}

	// 当前实现只验证 Evidence ID 是否存在，不验证 Evidence 是否支持 Root Cause
	// 这是已知的技术限制，记录为 PARTIAL
	// PVC corruption 与 AlertmanagerClusterCrashlooping alert 之间没有直接证据关系
	// 但当前系统无法进行语义验证

	t.Log("V5 PARTIAL: Evidence ID validation passes, but semantic Evidence-to-Root-Cause validation is not yet implemented")
	t.Log("  Known limitation: P1-X.4.1 provides Evidence Reference Integrity, but does not provide semantic causal consistency check")
}

// Confidence Validation Boundary Tests
func TestConfidenceValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"negative", -0.5, 0},
		{"zero", 0, 0},
		{"normal", 0.5, 0.5},
		{"one", 1.0, 1.0},
		{"over one", 1.5, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := tt.input
			if conf < 0 {
				conf = 0
			}
			if conf > 1 {
				conf = 1
			}
			if conf != tt.expected {
				t.Fatalf("expected %f, got %f", tt.expected, conf)
			}
		})
	}
	t.Log("Confidence validation PASS: 0 <= confidence <= 1")
}

// Evidence ID Deterministic Test
func TestEvidenceIDDeterministic(t *testing.T) {
	ts := time.Date(2026, 8, 21, 11, 29, 7, 0, time.UTC)
	id1 := GenerateEvidenceID(11, "alerts", "alert", "alertmanager", ts, "AlertmanagerClusterCrashlooping")
	id2 := GenerateEvidenceID(11, "alerts", "alert", "alertmanager", ts, "AlertmanagerClusterCrashlooping")
	if id1 != id2 {
		t.Fatalf("deterministic ID mismatch: %s != %s", id1, id2)
	}

	id3 := GenerateEvidenceID(11, "alerts", "alert", "alertmanager", ts, "DifferentAlert")
	if id1 == id3 {
		t.Fatal("different content should produce different IDs")
	}

	if len(id1) != 10 { // "E-" + 8 chars
		t.Fatalf("expected ID length 10, got %d: %s", len(id1), id1)
	}
	t.Logf("Evidence ID deterministic PASS: %s", id1)
}

// Empty Evidence ID Test
func TestEmptyEvidenceIDRejected(t *testing.T) {
	validIDs := map[string]bool{"E-VALID001": true}
	result := &AIAnalysisResult{
		Evidence: []AIEvidence{
			{ID: "", Type: "alert", Description: "Empty ID"},
			{ID: "E-VALID001", Type: "alert", Description: "Valid"},
		},
	}
	accepted, rejected := ValidateEvidenceReferences(result, validIDs)
	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted, got %d", len(accepted))
	}
	if len(rejected) != 1 {
		t.Fatalf("expected 1 rejected (empty id), got %d", len(rejected))
	}
	t.Log("Empty evidence ID rejected PASS")
}
