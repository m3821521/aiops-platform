package signals

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/topology"
)

// ============================================================================
// Mock implementations
// ============================================================================

type mockK8sReader struct {
	deployments    []appsv1.Deployment
	statefulSets   []appsv1.StatefulSet
	daemonSets     []appsv1.DaemonSet
	deployErr      error
	statefulErr    error
	daemonErr      error
}

func (m *mockK8sReader) ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error) {
	return m.deployments, m.deployErr
}
func (m *mockK8sReader) ListStatefulSets(ctx context.Context, cluster, namespace string) ([]appsv1.StatefulSet, error) {
	return m.statefulSets, m.statefulErr
}
func (m *mockK8sReader) ListDaemonSets(ctx context.Context, cluster, namespace string) ([]appsv1.DaemonSet, error) {
	return m.daemonSets, m.daemonErr
}

type mockPromQuerier struct {
	result *monitoring.QueryResult
	err    error
}

func (m *mockPromQuerier) Query(ctx context.Context, query string, ts time.Time) (*monitoring.QueryResult, error) {
	return m.result, m.err
}

type mockAlertLister struct {
	alerts []alert.Alert
	err    error
}

func (m *mockAlertLister) List(ctx context.Context, filter alert.ListFilter, page, pageSize int) ([]alert.Alert, int64, error) {
	return m.alerts, int64(len(m.alerts)), m.err
}

type mockLogSearcher struct {
	result *logging.SearchResult
	err    error
}

func (m *mockLogSearcher) Search(ctx context.Context, q logging.SearchQuery) (*logging.SearchResult, error) {
	return m.result, m.err
}

type mockTopologyGetter struct {
	graph *topology.Graph
	err   error
}

func (m *mockTopologyGetter) GetGraph(ctx context.Context, cluster, namespace string, refresh bool) (*topology.Graph, error) {
	return m.graph, m.err
}

// mockCollector 是一个通用的 mock SignalCollector，用于 Manager 测试。
type mockCollector struct {
	source    string
	evidences []rca.Evidence
	err       error
}

func (m *mockCollector) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error) {
	return m.evidences, m.err
}
func (m *mockCollector) Source() string { return m.source }

// ============================================================================
// Helper functions
// ============================================================================

func testServiceCtx() ServiceContext {
	return ServiceContext{
		ID:               1,
		Name:             "test-svc",
		Namespace:        "default",
		Cluster:          "test-cluster",
		WorkloadType:     "deployment",
		WorkloadName:     "test-deploy",
		WorkloadSelector: map[string]string{"app": "test"},
	}
}

func readyPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"app": "test"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 0},
			},
		},
	}
}

func crashLoopPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"app": "test"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        false,
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 5m0s"},
					},
				},
			},
		},
	}
}

func oomKilledPod(name string) corev1.Pod {
	finishedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"app": "test"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 1,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:     "OOMKilled",
							ExitCode:   137,
							FinishedAt: finishedAt,
						},
					},
				},
			},
		},
	}
}

func imagePullPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"app": "test"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "main",
					Ready: false,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "Back-off pulling image"},
					},
				},
			},
		},
	}
}

// ============================================================================
// Kubernetes Collector Tests
// ============================================================================

func TestKubernetesCollector_ReadyPod(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) == 0 {
		t.Fatal("expected at least one evidence for ready pod")
	}
	// Ready pod should produce pod_readiness with level=context
	found := false
	for _, e := range evidences {
		if e.Type == "pod_readiness" && e.Level == rca.EvidenceLevelContext {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pod_readiness context evidence for ready pod")
	}
}

func TestKubernetesCollector_NotReadyPod(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	notReady := readyPod("pod-1")
	notReady.Status.Conditions[0].Status = corev1.ConditionFalse
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{notReady})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range evidences {
		if e.Type == "pod_readiness" && e.Level == rca.EvidenceLevelCorroborating {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pod_readiness corroborating evidence for not-ready pod")
	}
}

func TestKubernetesCollector_CrashLoop(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{crashLoopPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range evidences {
		if e.Type == "crashloop" && e.Level == rca.EvidenceLevelCorroborating && e.CausalRelevance == "supporting" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected crashloop corroborating evidence")
	}
}

func TestKubernetesCollector_OOMKilled(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{oomKilledPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range evidences {
		if e.Type == "oomkilled" && e.Level == rca.EvidenceLevelDirect && e.CausalRelevance == "direct_causal" {
			found = true
			if e.DataTimestamp == nil {
				t.Error("OOMKilled evidence should have dataTimestamp")
			}
			break
		}
	}
	if !found {
		t.Error("expected oomkilled direct_causal evidence")
	}
}

func TestKubernetesCollector_ImagePull(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{imagePullPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range evidences {
		if e.Type == "image_pull" && e.Level == rca.EvidenceLevelDirect {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected image_pull direct evidence")
	}
}

func TestKubernetesCollector_RestartCount(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	pod := readyPod("pod-1")
	pod.Status.ContainerStatuses[0].RestartCount = 5
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{pod})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range evidences {
		if e.Type == "pod_restart" {
			found = true
			if e.Value != 5 {
				t.Errorf("expected restart count 5, got %v", e.Value)
			}
			break
		}
	}
	if !found {
		t.Error("expected pod_restart evidence")
	}
}

func TestKubernetesCollector_DeploymentAvailability(t *testing.T) {
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deploy", Namespace: "default"},
		Status: appsv1.DeploymentStatus{
			Replicas:          3,
			ReadyReplicas:     2,
			AvailableReplicas: 2,
		},
	}
	c := NewKubernetesSignalCollector(&mockK8sReader{deployments: []appsv1.Deployment{dep}}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range evidences {
		if e.Type == "deployment_availability" {
			found = true
			if e.Level != rca.EvidenceLevelCorroborating {
				t.Errorf("expected corroborating for unavailable deployment, got %s", e.Level)
			}
			break
		}
	}
	if !found {
		t.Error("expected deployment_availability evidence")
	}
}

func TestKubernetesCollector_EmptyPods(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty pods should not produce pod-level evidences, but may produce workload availability
	// This is acceptable - empty != error
	_ = evidences
}

func TestKubernetesCollector_ProvenanceComplete(t *testing.T) {
	c := NewKubernetesSignalCollector(&mockK8sReader{}, 3)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range evidences {
		if e.Source == "" {
			t.Error("evidence source is empty")
		}
		if e.SourceType == "" {
			t.Error("evidence sourceType is empty")
		}
		if e.FetchedAt == nil {
			t.Error("evidence fetchedAt is nil")
		}
		if e.TrustStatus == "" {
			t.Error("evidence trustStatus is empty")
		}
		if e.CausalRelevance == "" {
			t.Error("evidence causalRelevance is empty")
		}
	}
}

// ============================================================================
// Prometheus Collector Tests
// ============================================================================

func TestPrometheusCollector_NilQuerier(t *testing.T) {
	c := NewPrometheusSignalCollector(nil, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("nil querier should return empty (not error)")
	}
}

func TestPrometheusCollector_QueryError(t *testing.T) {
	c := NewPrometheusSignalCollector(&mockPromQuerier{err: errors.New("connection refused")}, 5*time.Minute)
	_, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err == nil {
		t.Error("expected error for query failure")
	}
}

func TestPrometheusCollector_EmptyResult(t *testing.T) {
	c := NewPrometheusSignalCollector(&mockPromQuerier{result: &monitoring.QueryResult{}}, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("empty result should return no evidences (empty != error)")
	}
}

func TestPrometheusCollector_NilResult(t *testing.T) {
	c := NewPrometheusSignalCollector(&mockPromQuerier{result: nil}, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("nil result should return no evidences")
	}
}

// ============================================================================
// Alert Collector Tests
// ============================================================================

func TestAlertCollector_NilLister(t *testing.T) {
	c := NewAlertSignalCollector(nil)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("nil lister should return empty")
	}
}

func TestAlertCollector_FiringAlert(t *testing.T) {
	c := NewAlertSignalCollector(&mockAlertLister{
		alerts: []alert.Alert{
			{Fingerprint: "fp1", Alertname: "HighErrorRate", Severity: alert.SeverityCritical, Status: alert.StatusFiring, Namespace: "default", Service: "test-svc", StartsAt: time.Now()},
		},
	})
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(evidences))
	}
	e := evidences[0]
	if e.Type != "alert_firing" {
		t.Errorf("expected alert_firing, got %s", e.Type)
	}
	if e.Level != rca.EvidenceLevelCorroborating {
		t.Errorf("critical alert should be corroborating, got %s", e.Level)
	}
	if e.CausalRelevance == "direct_causal" {
		t.Error("alert should never be direct_causal")
	}
}

func TestAlertCollector_ResolvedAlert(t *testing.T) {
	c := NewAlertSignalCollector(&mockAlertLister{
		alerts: []alert.Alert{
			{Fingerprint: "fp1", Alertname: "HighErrorRate", Status: alert.StatusResolved, Namespace: "default", Service: "test-svc"},
		},
	})
	// Note: our filter only queries firing alerts, so resolved won't be returned
	// But if it were, it should not produce active signal
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The mock returns resolved alert despite filter, but our code only processes
	// what's returned. This test verifies no crash.
	_ = evidences
}

func TestAlertCollector_NoServiceLabel(t *testing.T) {
	c := NewAlertSignalCollector(&mockAlertLister{
		alerts: []alert.Alert{
			{Fingerprint: "fp1", Alertname: "NodeDown", Severity: alert.SeverityWarning, Status: alert.StatusFiring, Namespace: "default"},
		},
	})
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Alert without service label and not matching pod should not be included
	if len(evidences) != 0 {
		t.Error("alert without service label should not match service")
	}
}

func TestAlertCollector_QueryError(t *testing.T) {
	c := NewAlertSignalCollector(&mockAlertLister{err: errors.New("db error")})
	_, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err == nil {
		t.Error("expected error for alert list failure")
	}
}

// ============================================================================
// Log Collector Tests
// ============================================================================

func TestLogCollector_NilES(t *testing.T) {
	c := NewLogSignalCollector(nil, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("nil ES should return empty (not error)")
	}
}

func TestLogCollector_QueryError(t *testing.T) {
	c := NewLogSignalCollector(&mockLogSearcher{err: errors.New("es unavailable")}, 5*time.Minute)
	_, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err == nil {
		t.Error("expected error for ES failure")
	}
}

func TestLogCollector_EmptyResult(t *testing.T) {
	c := NewLogSignalCollector(&mockLogSearcher{result: &logging.SearchResult{Total: 0}}, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("empty logs should return no evidences (empty != error)")
	}
}

func TestLogCollector_ErrorLogs(t *testing.T) {
	c := NewLogSignalCollector(&mockLogSearcher{
		result: &logging.SearchResult{
			Total: 2,
			Hits: []logging.LogHit{
				{Pod: "pod-1", Timestamp: time.Now(), Level: "error"},
				{Pod: "pod-1", Timestamp: time.Now(), Level: "error"},
			},
		},
	}, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 1 {
		t.Fatalf("expected 1 evidence (aggregated by pod), got %d", len(evidences))
	}
	e := evidences[0]
	if e.Type != "log_error_rate" {
		t.Errorf("expected log_error_rate, got %s", e.Type)
	}
	if e.Value != 2 {
		t.Errorf("expected value 2, got %v", e.Value)
	}
}

// ============================================================================
// Topology Collector Tests
// ============================================================================

func TestTopologyCollector_Nil(t *testing.T) {
	c := NewTopologySignalCollector(nil)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("nil topology should return empty")
	}
}

func TestTopologyCollector_QueryError(t *testing.T) {
	c := NewTopologySignalCollector(&mockTopologyGetter{err: errors.New("cache miss")})
	_, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err == nil {
		t.Error("expected error for topology failure")
	}
}

func TestTopologyCollector_HealthyService(t *testing.T) {
	svcNode := topology.Node{
		ID:     topology.NodeID("test-cluster", topology.TypeService, "default", "test-svc"),
		Type:   topology.TypeService,
		Name:   "test-svc",
		Status: topology.StatusHealthy,
	}
	c := NewTopologySignalCollector(&mockTopologyGetter{
		graph: &topology.Graph{Nodes: []topology.Node{svcNode}},
	})
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("healthy service should not produce dependency_health signal")
	}
}

func TestTopologyCollector_CriticalService(t *testing.T) {
	svcNode := topology.Node{
		ID:         topology.NodeID("test-cluster", topology.TypeService, "default", "test-svc"),
		Type:       topology.TypeService,
		Name:       "test-svc",
		Status:     topology.StatusCritical,
		AlertCount: 3,
	}
	c := NewTopologySignalCollector(&mockTopologyGetter{
		graph: &topology.Graph{Nodes: []topology.Node{svcNode}},
	})
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidences) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(evidences))
	}
	e := evidences[0]
	if e.Type != "dependency_health" {
		t.Errorf("expected dependency_health, got %s", e.Type)
	}
	if e.TimestampAvailable {
		t.Error("topology should have TimestampAvailable=false (no original event time)")
	}
}

// ============================================================================
// Manager Tests
// ============================================================================

func TestManager_PartialFailure(t *testing.T) {
	collectors := []SignalCollector{
		&mockCollector{source: "k8s", evidences: []rca.Evidence{{ID: "e1", Type: "pod_readiness"}}},
		&mockCollector{source: "prom", err: errors.New("prom down")},
	}
	mgr := NewSignalCollectorManager(collectors, 0)
	result, err := mgr.Collect(context.Background(), testServiceCtx(), nil)
	if err != nil {
		t.Fatalf("partial failure should not return error: %v", err)
	}
	if len(result.Signals) != 1 {
		t.Errorf("expected 1 signal from k8s, got %d", len(result.Signals))
	}
	if len(result.SourceErrors) != 1 {
		t.Errorf("expected 1 source_error, got %d", len(result.SourceErrors))
	}
	if result.SourceErrors["prom"] == "" {
		t.Error("expected prom source_error")
	}
}

func TestManager_AllFailure(t *testing.T) {
	collectors := []SignalCollector{
		&mockCollector{source: "k8s", err: errors.New("k8s down")},
		&mockCollector{source: "prom", err: errors.New("prom down")},
	}
	mgr := NewSignalCollectorManager(collectors, 0)
	result, err := mgr.Collect(context.Background(), testServiceCtx(), nil)
	if !errors.Is(err, ErrAllCollectorsFailed) {
		t.Fatalf("expected ErrAllCollectorsFailed, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result with source_errors")
	}
	if len(result.SourceErrors) != 2 {
		t.Errorf("expected 2 source_errors, got %d", len(result.SourceErrors))
	}
}

func TestManager_NoEvidence(t *testing.T) {
	collectors := []SignalCollector{
		&mockCollector{source: "k8s", evidences: nil},
		&mockCollector{source: "prom", evidences: nil},
	}
	mgr := NewSignalCollectorManager(collectors, 0)
	result, err := mgr.Collect(context.Background(), testServiceCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Signals) != 0 {
		t.Error("expected no signals")
	}
	if len(result.SourceErrors) != 0 {
		t.Error("expected no source_errors")
	}
	// No evidence is different from all failure
	if result.AllFailed() {
		t.Error("no evidence should not be all failure")
	}
}

func TestManager_EmptyCollectors(t *testing.T) {
	mgr := NewSignalCollectorManager(nil, 0)
	result, err := mgr.Collect(context.Background(), testServiceCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestManager_CollectForService(t *testing.T) {
	collectors := []SignalCollector{
		&mockCollector{source: "k8s", evidences: []rca.Evidence{{ID: "e1"}}},
	}
	mgr := NewSignalCollectorManager(collectors, 0)
	result, err := mgr.CollectForService(
		context.Background(),
		1, "test-svc", "default", "test-cluster",
		"deployment", "test-deploy",
		map[string]string{"app": "test"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(result.Signals))
	}
}

// ============================================================================
// Error != Empty Semantics Tests
// ============================================================================

func TestErrorIsNotEmpty(t *testing.T) {
	// Prometheus query error should NOT produce value=0 evidence
	c := NewPrometheusSignalCollector(&mockPromQuerier{err: errors.New("down")}, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(evidences) != 0 {
		t.Error("error should not produce fake evidences")
	}
}

func TestEmptyIsNotError(t *testing.T) {
	// Prometheus empty result should NOT return error
	c := NewPrometheusSignalCollector(&mockPromQuerier{result: &monitoring.QueryResult{}}, 5*time.Minute)
	evidences, err := c.Collect(context.Background(), testServiceCtx(), []corev1.Pod{readyPod("pod-1")})
	if err != nil {
		t.Fatalf("empty result should not be error: %v", err)
	}
	if len(evidences) != 0 {
		t.Error("empty result should have no evidences")
	}
}

func TestNoFakeHealthy(t *testing.T) {
	// All collectors failing should not produce healthy signals
	collectors := []SignalCollector{
		&mockCollector{source: "k8s", err: errors.New("down")},
	}
	mgr := NewSignalCollectorManager(collectors, 0)
	result, err := mgr.Collect(context.Background(), testServiceCtx(), nil)
	if !errors.Is(err, ErrAllCollectorsFailed) {
		t.Fatal("expected all failure")
	}
	if result.HasSignals() {
		t.Error("all failure should not have signals")
	}
}
