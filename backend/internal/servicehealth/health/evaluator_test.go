package health

import (
	"fmt"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/rca"
)

// ============================================================================
// Test helpers
// ============================================================================

func freshEvidence(evType string, level rca.EvidenceLevel, expected string, value float64) rca.Evidence {
	return rca.Evidence{
		ID:            "ev-" + evType + "-1",
		Type:          rca.EvidenceType(evType),
		Level:         level,
		Source:        "kubernetes",
		SourceType:    "provider",
		TrustStatus:   "fresh",
		CausalRelevance: "contextual",
		Expected:      expected,
		Value:         value,
		Description:   "test evidence",
		FetchedAt:     timePtr(time.Now()),
		DataTimestamp: timePtr(time.Now()),
	}
}

func staleEvidence(evType string) rca.Evidence {
	e := freshEvidence(evType, rca.EvidenceLevelContext, "", 0)
	e.TrustStatus = "stale"
	e.ID = "ev-stale-" + evType
	return e
}

func errorEvidence(evType string) rca.Evidence {
	e := freshEvidence(evType, rca.EvidenceLevelContext, "", 0)
	e.TrustStatus = "error"
	e.ID = "ev-error-" + evType
	return e
}

func emptyEvidence(evType string) rca.Evidence {
	e := freshEvidence(evType, rca.EvidenceLevelContext, "", 0)
	e.TrustStatus = "empty"
	e.ID = "ev-empty-" + evType
	return e
}

func deploymentAvailability(ready, desired int) rca.Evidence {
	return freshEvidence("deployment_availability", rca.EvidenceLevelContext,
		formatAvailability(ready, desired, "replicas"), float64(ready))
}

func statefulSetAvailability(ready, desired int) rca.Evidence {
	return freshEvidence("statefulset_availability", rca.EvidenceLevelContext,
		formatAvailability(ready, desired, "replicas"), float64(ready))
}

func daemonSetAvailability(ready, desired int) rca.Evidence {
	return freshEvidence("daemonset_availability", rca.EvidenceLevelContext,
		formatAvailability(ready, desired, "nodes"), float64(ready))
}

func formatAvailability(ready, desired int, unit string) string {
	return fmt.Sprintf("%d/%d %s ready", ready, desired, unit)
}

// podReadinessReady 产生一个正常的 pod_readiness evidence。
func podReadinessReady() rca.Evidence {
	return freshEvidence("pod_readiness", rca.EvidenceLevelContext, "", 1)
}

// podReadinessNotReady 产生一个 NotReady 的 pod_readiness evidence。
func podReadinessNotReady() rca.Evidence {
	e := freshEvidence("pod_readiness", rca.EvidenceLevelCorroborating, "", 0)
	e.ID = "ev-pod-readiness-notready-1"
	return e
}

// crashLoopEvidence 产生一个 CrashLoopBackOff evidence。
func crashLoopEvidence() rca.Evidence {
	e := freshEvidence("crashloop", rca.EvidenceLevelCorroborating, "", 1)
	e.CausalRelevance = "supporting"
	e.ID = "ev-crashloop-1"
	return e
}

// oomKilledEvidence 产生一个 OOMKilled evidence。
func oomKilledEvidence() rca.Evidence {
	e := freshEvidence("oomkilled", rca.EvidenceLevelDirect, "", 137)
	e.CausalRelevance = "direct_causal"
	e.ID = "ev-oomkilled-1"
	return e
}

// imagePullEvidence 产生一个 ImagePullBackOff evidence。
func imagePullEvidence() rca.Evidence {
	e := freshEvidence("image_pull", rca.EvidenceLevelDirect, "", 1)
	e.CausalRelevance = "direct_causal"
	e.ID = "ev-imagepull-1"
	return e
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// ============================================================================
// Table-driven tests
// ============================================================================

func TestHealthEvaluator(t *testing.T) {
	tests := []struct {
		name      string
		evidences []rca.Evidence
		wantState HealthState
	}{
		// 1. all healthy => Healthy
		{
			name:      "all healthy deployment",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), podReadinessReady()},
			wantState: HealthStateHealthy,
		},
		// 2. one NotReady => Degraded
		{
			name:      "one NotReady pod",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), podReadinessNotReady()},
			wantState: HealthStateDegraded,
		},
		// 3. CrashLoop + remaining replicas ready => Degraded
		{
			name:      "CrashLoop with remaining ready",
			evidences: []rca.Evidence{deploymentAvailability(2, 3), crashLoopEvidence()},
			wantState: HealthStateDegraded,
		},
		// 4. OOMKilled + remaining replicas ready => Degraded
		{
			name:      "OOMKilled with remaining ready",
			evidences: []rca.Evidence{deploymentAvailability(2, 3), oomKilledEvidence()},
			wantState: HealthStateDegraded,
		},
		// 5. ImagePullBackOff => Degraded
		{
			name:      "ImagePullBackOff",
			evidences: []rca.Evidence{deploymentAvailability(0, 3), imagePullEvidence()},
			wantState: HealthStateDown, // availability=0 => Down
		},
		// 6. Deployment available=0 => Down
		{
			name:      "Deployment available=0",
			evidences: []rca.Evidence{deploymentAvailability(0, 3)},
			wantState: HealthStateDown,
		},
		// 7. StatefulSet ready=0 => Down
		{
			name:      "StatefulSet ready=0",
			evidences: []rca.Evidence{statefulSetAvailability(0, 3)},
			wantState: HealthStateDown,
		},
		// 8. DaemonSet available=0 => Down
		{
			name:      "DaemonSet available=0",
			evidences: []rca.Evidence{daemonSetAvailability(0, 3)},
			wantState: HealthStateDown,
		},
		// 9. partial availability => Degraded
		{
			name:      "partial availability 2/3",
			evidences: []rca.Evidence{deploymentAvailability(2, 3)},
			wantState: HealthStateDegraded,
		},
		// 10. all fresh evidence healthy => Healthy
		{
			name:      "all fresh healthy",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), podReadinessReady(), freshEvidence("prometheus_error_rate", rca.EvidenceLevelContext, "", 0)},
			wantState: HealthStateHealthy,
		},
		// 11. stale-only => Unknown
		{
			name:      "stale-only",
			evidences: []rca.Evidence{staleEvidence("deployment_availability"), staleEvidence("pod_readiness")},
			wantState: HealthStateUnknown,
		},
		// 12. error-only => Unknown
		{
			name:      "error-only",
			evidences: []rca.Evidence{errorEvidence("prometheus_error_rate"), errorEvidence("log_error_rate")},
			wantState: HealthStateUnknown,
		},
		// 13. empty-only => Unknown
		{
			name:      "empty-only",
			evidences: []rca.Evidence{emptyEvidence("prometheus_error_rate"), emptyEvidence("alert_firing")},
			wantState: HealthStateUnknown,
		},
		// 14. mixed healthy + stale => Healthy
		{
			name:      "mixed healthy + stale",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), staleEvidence("prometheus_error_rate")},
			wantState: HealthStateHealthy,
		},
		// 15. mixed healthy + error => Healthy
		{
			name:      "mixed healthy + error",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), errorEvidence("prometheus_error_rate")},
			wantState: HealthStateHealthy,
		},
		// 16. no evidence => Unknown
		{
			name:      "no evidence",
			evidences: []rca.Evidence{},
			wantState: HealthStateUnknown,
		},
		// 17. Down outranks Degraded
		{
			name:      "Down outranks Degraded",
			evidences: []rca.Evidence{deploymentAvailability(0, 3), crashLoopEvidence()},
			wantState: HealthStateDown,
		},
		// 18. Degraded outranks Healthy
		{
			name:      "Degraded outranks Healthy",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), crashLoopEvidence()},
			wantState: HealthStateDegraded,
		},
		// 19. single OOMKilled cannot directly cause Down
		{
			name:      "single OOMKilled not Down",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), oomKilledEvidence()},
			wantState: HealthStateDegraded, // 不是 Down
		},
		// 20. single CrashLoop cannot directly cause Down
		{
			name:      "single CrashLoop not Down",
			evidences: []rca.Evidence{deploymentAvailability(3, 3), crashLoopEvidence()},
			wantState: HealthStateDegraded, // 不是 Down
		},
		// 21. EvidenceIDs correctly retained
		{
			name:      "EvidenceIDs retained",
			evidences: []rca.Evidence{deploymentAvailability(2, 3), crashLoopEvidence()},
			wantState: HealthStateDegraded,
		},
		// 22. EvaluatedAt non-zero (tested below)
		// 23. no fake healthy (stale-only => Unknown, already tested #11)
		// 24. no mutation of input Evidence (tested below)
	}

	evaluator := NewDefaultEvaluator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 24. 保存输入快照，验证不被修改
			originalIDs := make([]string, len(tt.evidences))
			originalTypes := make([]string, len(tt.evidences))
			originalTrust := make([]string, len(tt.evidences))
			for i, e := range tt.evidences {
				originalIDs[i] = e.ID
				originalTypes[i] = string(e.Type)
				originalTrust[i] = e.TrustStatus
			}

			result := evaluator.Evaluate(tt.evidences)

			if result.State != tt.wantState {
				t.Errorf("state = %v, want %v", result.State, tt.wantState)
			}

			// 22. EvaluatedAt non-zero
			if result.EvaluatedAt.IsZero() {
				t.Error("EvaluatedAt is zero")
			}

			// 21. EvidenceIDs correctly retained (non-empty when there are evidences)
			if len(tt.evidences) > 0 && len(result.EvidenceIDs) == 0 {
				t.Error("EvidenceIDs is empty but there were input evidences")
			}

			// 24. 验证输入未被修改
			for i, e := range tt.evidences {
				if e.ID != originalIDs[i] {
					t.Errorf("input Evidence[%d].ID mutated: %s -> %s", i, originalIDs[i], e.ID)
				}
				if string(e.Type) != originalTypes[i] {
					t.Errorf("input Evidence[%d].Type mutated", i)
				}
				if e.TrustStatus != originalTrust[i] {
					t.Errorf("input Evidence[%d].TrustStatus mutated", i)
				}
			}
		})
	}
}

// ============================================================================
// Additional specific tests
// ============================================================================

func TestReasonForUnknown(t *testing.T) {
	evaluator := NewDefaultEvaluator()

	// no evidence
	result := evaluator.Evaluate(nil)
	if result.State != HealthStateUnknown {
		t.Errorf("nil evidence => %v, want Unknown", result.State)
	}
	if result.Reason == "" {
		t.Error("Reason is empty for Unknown")
	}
}

func TestParseAvailability(t *testing.T) {
	tests := []struct {
		input   string
		want    availabilityInfo
	}{
		{"2/3 replicas ready (2 available)", availabilityInfo{ready: 2, desired: 3, valid: true}},
		{"0/3 replicas ready", availabilityInfo{ready: 0, desired: 3, valid: true}},
		{"3/3 nodes ready", availabilityInfo{ready: 3, desired: 3, valid: true}},
		{"", availabilityInfo{}},
		{"invalid", availabilityInfo{}},
		{"2/3", availabilityInfo{ready: 2, desired: 3, valid: true}},
	}

	for _, tt := range tests {
		got := parseAvailability(tt.input)
		if got != tt.want {
			t.Errorf("parseAvailability(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestMergeState(t *testing.T) {
	tests := []struct {
		a, b HealthState
		want HealthState
	}{
		{HealthStateDown, HealthStateDegraded, HealthStateDown},
		{HealthStateDegraded, HealthStateHealthy, HealthStateDegraded},
		{HealthStateHealthy, HealthStateUnknown, HealthStateHealthy},
		{HealthStateUnknown, HealthStateHealthy, HealthStateHealthy},
		{HealthStateUnknown, HealthStateUnknown, HealthStateUnknown},
		{HealthStateDown, HealthStateHealthy, HealthStateDown},
	}

	for _, tt := range tests {
		got := mergeState(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("mergeState(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNoFakeHealthyFromError(t *testing.T) {
	evaluator := NewDefaultEvaluator()
	// 只有 error evidence，不能是 Healthy
	result := evaluator.Evaluate([]rca.Evidence{
		errorEvidence("prometheus_error_rate"),
		errorEvidence("log_error_rate"),
	})
	if result.State == HealthStateHealthy {
		t.Error("error-only evidence should not be Healthy (no fake healthy)")
	}
	if result.State != HealthStateUnknown {
		t.Errorf("error-only => %v, want Unknown", result.State)
	}
}

func TestNoFakeHealthyFromStale(t *testing.T) {
	evaluator := NewDefaultEvaluator()
	// 只有 stale evidence，不能是 Healthy
	result := evaluator.Evaluate([]rca.Evidence{
		staleEvidence("deployment_availability"),
	})
	if result.State == HealthStateHealthy {
		t.Error("stale-only evidence should not be Healthy (no fake healthy)")
	}
}

func TestDownWithPodAbnormal(t *testing.T) {
	// availability=0 + OOMKilled => Down (不是 Degraded)
	evaluator := NewDefaultEvaluator()
	result := evaluator.Evaluate([]rca.Evidence{
		deploymentAvailability(0, 3),
		oomKilledEvidence(),
	})
	if result.State != HealthStateDown {
		t.Errorf("availability=0 + OOMKilled => %v, want Down", result.State)
	}
}

func TestDegradedWithPartialAvailabilityAndOOM(t *testing.T) {
	// availability=2/3 + OOMKilled => Degraded
	evaluator := NewDefaultEvaluator()
	result := evaluator.Evaluate([]rca.Evidence{
		deploymentAvailability(2, 3),
		oomKilledEvidence(),
	})
	if result.State != HealthStateDegraded {
		t.Errorf("partial availability + OOMKilled => %v, want Degraded", result.State)
	}
}
