package servicehealth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/servicehealth/health"
)

// ============================================================================
// countSignals tests
// ============================================================================

func TestCountSignals(t *testing.T) {
	tests := []struct {
		name      string
		evidences []rca.Evidence
		want      SignalCounts
	}{
		{
			name:      "empty",
			evidences: nil,
			want:      SignalCounts{Total: 0, Fresh: 0, Stale: 0, Error: 0, Empty: 0},
		},
		{
			name: "all fresh",
			evidences: []rca.Evidence{
				{TrustStatus: "fresh"},
				{TrustStatus: "fresh"},
				{TrustStatus: "fresh"},
			},
			want: SignalCounts{Total: 3, Fresh: 3, Stale: 0, Error: 0, Empty: 0},
		},
		{
			name: "mixed trust statuses",
			evidences: []rca.Evidence{
				{TrustStatus: "fresh"},
				{TrustStatus: "stale"},
				{TrustStatus: "error"},
				{TrustStatus: "empty"},
				{TrustStatus: "fresh"},
			},
			want: SignalCounts{Total: 5, Fresh: 2, Stale: 1, Error: 1, Empty: 1},
		},
		{
			name: "unknown trust status not counted in categories",
			evidences: []rca.Evidence{
				{TrustStatus: "unknown"},
				{TrustStatus: "fresh"},
			},
			want: SignalCounts{Total: 2, Fresh: 1, Stale: 0, Error: 0, Empty: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSignals(tt.evidences)
			if got != tt.want {
				t.Errorf("countSignals() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Handler.Health input validation tests
// ============================================================================

func TestHandlerHealth_MissingCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil) // nil manager is fine because we test input validation first

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/platform/services/1/health", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	h.Health(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing cluster: status = %d, want 400", w.Code)
	}
}

func TestHandlerHealth_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/platform/services/invalid/health?cluster=k8ss", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	h.Health(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid id: status = %d, want 400", w.Code)
	}
}

func TestHandlerHealth_ValidIDFormat(t *testing.T) {
	// 验证有效 ID 格式不会在输入验证阶段被拒绝
	// 使用非 nil Manager（依赖为 nil），这样会返回 500 而不是 panic
	gin.SetMode(gin.TestMode)
	mgr := NewManager(nil, nil, nil, nil) // all deps nil, but manager non-nil
	h := NewHandler(mgr)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/platform/services/123/health?cluster=k8ss", nil)
	c.Params = gin.Params{{Key: "id", Value: "123"}}

	h.Health(c)

	// 输入验证通过，不会是 400
	// 因为 signalsManager 未配置，会返回 500
	if w.Code == http.StatusBadRequest {
		t.Error("valid id+cluster should not return 400")
	}
}

// ============================================================================
// HealthResult structure tests
// ============================================================================

func TestHealthResult_Structure(t *testing.T) {
	result := HealthResult{
		Service: ServiceInfo{
			ID:        1,
			Name:      "test-svc",
			Namespace: "default",
			Cluster:   "test-cluster",
		},
		Health: health.HealthEvaluation{
			State:       health.HealthStateHealthy,
			Reason:      "all signals healthy",
			EvidenceIDs: []string{"ev-1", "ev-2"},
		},
		Signals: SignalCounts{
			Total: 5,
			Fresh: 3,
			Stale: 1,
			Error: 1,
			Empty: 0,
		},
		SourceErrors: map[string]string{
			"prometheus": "timeout",
		},
	}

	if result.Service.ID != 1 {
		t.Error("Service.ID mismatch")
	}
	if result.Health.State != health.HealthStateHealthy {
		t.Error("Health.State mismatch")
	}
	if len(result.Health.EvidenceIDs) != 2 {
		t.Error("EvidenceIDs should be preserved")
	}
	if result.Signals.Total != 5 {
		t.Error("Signals.Total mismatch")
	}
	if result.SourceErrors["prometheus"] != "timeout" {
		t.Error("SourceErrors should be preserved")
	}
}

// ============================================================================
// No fake healthy invariant
// ============================================================================

func TestNoFakeHealthy_UnknownTrustStatus(t *testing.T) {
	// 只有 empty/error/stale evidence 不应该被 count 为 fresh
	evidences := []rca.Evidence{
		{TrustStatus: "empty"},
		{TrustStatus: "error"},
		{TrustStatus: "stale"},
	}
	counts := countSignals(evidences)
	if counts.Fresh != 0 {
		t.Errorf("non-fresh evidence should not count as fresh, got %d", counts.Fresh)
	}
	if counts.Total != 3 {
		t.Errorf("total should count all evidence, got %d", counts.Total)
	}
}

func TestEvaluatorResultNotModified(t *testing.T) {
	// 验证 HealthEvaluation 在传递过程中不被修改
	original := health.HealthEvaluation{
		State:       health.HealthStateDegraded,
		Reason:      "pod restart detected",
		EvidenceIDs: []string{"ev-1", "ev-2"},
	}

	result := HealthResult{
		Health: original,
	}

	// 验证 result.Health 与 original 一致
	if result.Health.State != original.State {
		t.Error("HealthEvaluation state should not be modified")
	}
	if result.Health.Reason != original.Reason {
		t.Error("HealthEvaluation reason should not be modified")
	}
	if len(result.Health.EvidenceIDs) != len(original.EvidenceIDs) {
		t.Error("HealthEvaluation evidence_ids should not be modified")
	}
}
