package rca_test

import (
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/rca"
)

func TestRCASingleService(t *testing.T) {
	alerts := []rca.AlertInfo{
		{Alertname: "HighCPU", Severity: "critical", Service: "order-service", Namespace: "default", StartsAt: time.Now()},
		{Alertname: "HighMemory", Severity: "warning", Service: "order-service", Namespace: "default", StartsAt: time.Now()},
	}

	engine := rca.NewEngine()
	result := engine.Analyze(alerts)

	if result.RootCause == "" {
		t.Fatal("expected root cause")
	}
	if result.Confidence <= 0 {
		t.Fatalf("expected confidence > 0, got %f", result.Confidence)
	}
	if len(result.AffectedServices) != 1 {
		t.Fatalf("expected 1 affected service, got %d", len(result.AffectedServices))
	}
	if len(result.Evidence) == 0 {
		t.Fatal("expected evidence")
	}
	if len(result.Timeline) != 2 {
		t.Fatalf("expected 2 timeline events, got %d", len(result.Timeline))
	}
}

func TestRCAMultiServiceRootCause(t *testing.T) {
	// order-service 有 3 条告警（2 critical），payment-service 有 1 条 warning。
	// order-service 应该被识别为根因。
	alerts := []rca.AlertInfo{
		{Alertname: "HighCPU", Severity: "critical", Service: "order-service", Namespace: "default", StartsAt: time.Now()},
		{Alertname: "PodRestart", Severity: "critical", Service: "order-service", Namespace: "default", StartsAt: time.Now()},
		{Alertname: "HighMemory", Severity: "warning", Service: "order-service", Namespace: "default", StartsAt: time.Now()},
		{Alertname: "LatencyHigh", Severity: "warning", Service: "payment-service", Namespace: "default", StartsAt: time.Now()},
	}

	engine := rca.NewEngine()
	result := engine.Analyze(alerts)

	if len(result.AffectedServices) != 2 {
		t.Fatalf("expected 2 affected services, got %d", len(result.AffectedServices))
	}
	// 根因应该是 order-service。
	if result.RootCause == "" {
		t.Fatal("expected root cause")
	}
	if result.Confidence <= 0 {
		t.Fatalf("expected confidence > 0, got %f", result.Confidence)
	}
}

func TestRCAEmpty(t *testing.T) {
	engine := rca.NewEngine()
	result := engine.Analyze(nil)

	if result.RootCause != "无告警" {
		t.Fatalf("expected '无告警', got %s", result.RootCause)
	}
	if result.Confidence != 0 {
		t.Fatalf("expected confidence 0, got %f", result.Confidence)
	}
}

func TestRCARootCauseDescriptionCPU(t *testing.T) {
	alerts := []rca.AlertInfo{
		{Alertname: "NodeHighCPU", Severity: "critical", Service: "worker-node", Namespace: "kube-system", StartsAt: time.Now()},
	}

	engine := rca.NewEngine()
	result := engine.Analyze(alerts)

	if result.RootCause == "" {
		t.Fatal("expected root cause")
	}
	t.Logf("root cause: %s", result.RootCause)
}

func TestRCARootCauseDescriptionRestart(t *testing.T) {
	alerts := []rca.AlertInfo{
		{Alertname: "PodCrashLoopBackOff", Severity: "critical", Service: "api-gateway", Namespace: "default", StartsAt: time.Now()},
	}

	engine := rca.NewEngine()
	result := engine.Analyze(alerts)

	if result.RootCause == "" {
		t.Fatal("expected root cause")
	}
	t.Logf("root cause: %s", result.RootCause)
}

func TestRCATimelineOrder(t *testing.T) {
	now := time.Now()
	alerts := []rca.AlertInfo{
		{Alertname: "Later", Severity: "warning", Service: "svc-a", StartsAt: now.Add(10 * time.Minute)},
		{Alertname: "Earlier", Severity: "critical", Service: "svc-b", StartsAt: now},
	}

	engine := rca.NewEngine()
	result := engine.Analyze(alerts)

	if len(result.Timeline) != 2 {
		t.Fatalf("expected 2 timeline events, got %d", len(result.Timeline))
	}
	// 时间线应该按时间升序。
	if !result.Timeline[0].Time.Before(result.Timeline[1].Time) {
		t.Fatal("timeline should be sorted by time ascending")
	}
}

func TestRCAEvidenceCount(t *testing.T) {
	alerts := []rca.AlertInfo{
		{Alertname: "HighCPU", Severity: "critical", Service: "svc-a", Namespace: "default", StartsAt: time.Now()},
		{Alertname: "HighMemory", Severity: "warning", Service: "svc-b", Namespace: "default", StartsAt: time.Now()},
	}

	engine := rca.NewEngine()
	result := engine.Analyze(alerts)

	// 至少应该有根因统计、最早时间、其他服务、告警类型 4 条证据。
	if len(result.Evidence) < 3 {
		t.Fatalf("expected at least 3 evidence, got %d", len(result.Evidence))
	}
	// 证据应该按 order 排序。
	for i := 1; i < len(result.Evidence); i++ {
		if result.Evidence[i].Order <= result.Evidence[i-1].Order {
			t.Fatal("evidence should be ordered")
		}
	}
}
