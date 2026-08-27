package servicehealth

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockK8s 是 KubernetesDiscovery 接口的测试 mock。
type mockK8s struct {
	services     []corev1.Service
	deployments  []appsv1.Deployment
	statefulSets []appsv1.StatefulSet
	daemonSets   []appsv1.DaemonSet
	pods         []corev1.Pod
	err          error
	callCount    map[string]int
}

func newMockK8s() *mockK8s {
	return &mockK8s{callCount: make(map[string]int)}
}

func (m *mockK8s) ListServices(ctx context.Context, cluster, namespace string) ([]corev1.Service, error) {
	m.callCount["ListServices"]++
	if m.err != nil {
		return nil, m.err
	}
	return m.services, nil
}

func (m *mockK8s) ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error) {
	m.callCount["ListDeployments"]++
	if m.err != nil {
		return nil, m.err
	}
	return m.deployments, nil
}

func (m *mockK8s) ListStatefulSets(ctx context.Context, cluster, namespace string) ([]appsv1.StatefulSet, error) {
	m.callCount["ListStatefulSets"]++
	if m.err != nil {
		return nil, m.err
	}
	return m.statefulSets, nil
}

func (m *mockK8s) ListDaemonSets(ctx context.Context, cluster, namespace string) ([]appsv1.DaemonSet, error) {
	m.callCount["ListDaemonSets"]++
	if m.err != nil {
		return nil, m.err
	}
	return m.daemonSets, nil
}

func (m *mockK8s) ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error) {
	m.callCount["ListPods"]++
	if m.err != nil {
		return nil, m.err
	}
	return m.pods, nil
}

// helper: 创建带 selector 的 Service
func svcWithSelector(name, namespace string, selector map[string]string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.ServiceSpec{Selector: selector, Type: corev1.ServiceTypeClusterIP},
	}
}

// helper: 创建带 OwnerReference 的 Pod
func podWithOwner(name, namespace string, labels map[string]string, ownerKind, ownerName string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: ownerKind, Name: ownerName},
			},
		},
	}
}

// TestSelectorToPod 验证 Service Selector 与 Pod Labels 匹配。
func TestSelectorToPod(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api"}),
	}
	mock.pods = []corev1.Pod{
		podWithOwner("api-abc", "default", map[string]string{"app": "api"}, "ReplicaSet", "api-12345"),
		podWithOwner("web-xyz", "default", map[string]string{"app": "web"}, "ReplicaSet", "web-67890"),
	}
	mock.deployments = []appsv1.Deployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "test-cluster", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 service, got %d", len(result))
	}
	if result[0].PodCount != 1 {
		t.Errorf("expected PodCount=1, got %d", result[0].PodCount)
	}
	if result[0].WorkloadType != WorkloadTypeDeployment {
		t.Errorf("expected WorkloadType=deployment, got %s", result[0].WorkloadType)
	}
	if result[0].WorkloadName != "api" {
		t.Errorf("expected WorkloadName=api, got %s", result[0].WorkloadName)
	}
}

// TestPodToDeployment 验证 Pod OwnerReference=ReplicaSet → Deployment 映射。
func TestPodToDeployment(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("web", "default", map[string]string{"app": "web"}),
	}
	mock.pods = []corev1.Pod{
		podWithOwner("web-xyz", "default", map[string]string{"app": "web"}, "ReplicaSet", "web-67890abc"),
	}
	mock.deployments = []appsv1.Deployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].WorkloadType != WorkloadTypeDeployment {
		t.Errorf("expected deployment, got %s", result[0].WorkloadType)
	}
	if result[0].WorkloadName != "web" {
		t.Errorf("expected web, got %s", result[0].WorkloadName)
	}
}

// TestPodToStatefulSet 验证 Pod OwnerReference=StatefulSet 直接匹配。
func TestPodToStatefulSet(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("db", "default", map[string]string{"app": "db"}),
	}
	mock.pods = []corev1.Pod{
		podWithOwner("db-0", "default", map[string]string{"app": "db"}, "StatefulSet", "db"),
	}
	mock.statefulSets = []appsv1.StatefulSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].WorkloadType != WorkloadTypeStatefulSet {
		t.Errorf("expected statefulset, got %s", result[0].WorkloadType)
	}
	if result[0].WorkloadName != "db" {
		t.Errorf("expected db, got %s", result[0].WorkloadName)
	}
}

// TestPodToDaemonSet 验证 Pod OwnerReference=DaemonSet 直接匹配。
func TestPodToDaemonSet(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("monitor", "monitoring", map[string]string{"app": "node-exporter"}),
	}
	mock.pods = []corev1.Pod{
		podWithOwner("node-exporter-abc", "monitoring", map[string]string{"app": "node-exporter"}, "DaemonSet", "node-exporter"),
	}
	mock.daemonSets = []appsv1.DaemonSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-exporter", Namespace: "monitoring"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "monitoring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].WorkloadType != WorkloadTypeDaemonSet {
		t.Errorf("expected daemonset, got %s", result[0].WorkloadType)
	}
}

// TestNoSelectorService 验证无 selector Service (ExternalName/Headless) 不绑定 workload。
func TestNoSelectorService(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "external-db", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName},
		},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].WorkloadType != WorkloadTypeUnknown {
		t.Errorf("expected unknown for no-selector service, got %s", result[0].WorkloadType)
	}
	if result[0].ServiceType != "ExternalName" {
		t.Errorf("expected ExternalName, got %s", result[0].ServiceType)
	}
}

// TestUnableToDetermineWorkload 验证有 selector 但无法匹配 workload 时为 unknown。
func TestUnableToDetermineWorkload(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("orphan", "default", map[string]string{"app": "orphan"}),
	}
	mock.pods = []corev1.Pod{
		// Pod 匹配 selector 但没有 OwnerReference
		{ObjectMeta: metav1.ObjectMeta{Name: "orphan-pod", Namespace: "default", Labels: map[string]string{"app": "orphan"}}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].WorkloadType != WorkloadTypeUnknown {
		t.Errorf("expected unknown when no owner reference, got %s", result[0].WorkloadType)
	}
	if result[0].PodCount != 1 {
		t.Errorf("expected PodCount=1, got %d", result[0].PodCount)
	}
}

// TestNamespaceFiltering 验证 namespace 参数传递正确。
func TestNamespaceFiltering(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "monitoring", map[string]string{"app": "api"}),
	}

	ds := NewDiscoveryService(mock)
	_, err := ds.Discover(context.Background(), "c1", "monitoring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mock 不验证 namespace 参数，但确保调用不报错
}

// TestClusterFiltering 验证 cluster 参数传递正确。
func TestClusterFiltering(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api"}),
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "prod-cluster", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Cluster != "prod-cluster" {
		t.Errorf("expected cluster=prod-cluster, got %s", result[0].Cluster)
	}
}

// TestEmptyDiscovery 验证 K8s 正常但无 Service 时返回空列表（非 error）。
func TestEmptyDiscovery(t *testing.T) {
	mock := newMockK8s()
	// 所有列表为空

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "empty-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

// TestK8sAPIError 验证 K8s API 失败时返回 error（不返回空列表）。
func TestK8sAPIError(t *testing.T) {
	mock := newMockK8s()
	mock.err = context.DeadlineExceeded

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %v", result)
	}
}

// TestErrorNotEqualsEmpty 验证 Error != Empty：API 失败不能返回空列表。
func TestErrorNotEqualsEmpty(t *testing.T) {
	// 空 discovery
	mockEmpty := newMockK8s()
	dsEmpty := NewDiscoveryService(mockEmpty)
	resultEmpty, errEmpty := dsEmpty.Discover(context.Background(), "c1", "ns")
	if errEmpty != nil || len(resultEmpty) != 0 {
		t.Errorf("empty discovery should return [], nil; got %v, %v", resultEmpty, errEmpty)
	}

	// error discovery
	mockErr := newMockK8s()
	mockErr.err = context.DeadlineExceeded
	dsErr := NewDiscoveryService(mockErr)
	resultErr, errErr := dsErr.Discover(context.Background(), "c1", "ns")
	if errErr == nil {
		t.Error("error discovery should return error")
	}
	if resultErr != nil {
		t.Error("error discovery should return nil result")
	}
}

// TestMultiKeySelector 验证多 key selector 全部匹配。
func TestMultiKeySelector(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api", "tier": "backend"}),
	}
	mock.pods = []corev1.Pod{
		// 完全匹配
		podWithOwner("api-1", "default", map[string]string{"app": "api", "tier": "backend"}, "ReplicaSet", "api-abc"),
		// 部分匹配（缺 tier）
		podWithOwner("api-2", "default", map[string]string{"app": "api"}, "ReplicaSet", "api-def"),
	}
	mock.deployments = []appsv1.Deployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].PodCount != 1 {
		t.Errorf("expected PodCount=1 (only full match), got %d", result[0].PodCount)
	}
}

// TestSelectorPartialMismatch 验证 selector 有一个 key 不匹配则整体不匹配。
func TestSelectorPartialMismatch(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api", "env": "prod"}),
	}
	mock.pods = []corev1.Pod{
		// app 匹配但 env 不匹配
		podWithOwner("api-staging", "default", map[string]string{"app": "api", "env": "staging"}, "ReplicaSet", "api-abc"),
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].PodCount != 0 {
		t.Errorf("expected PodCount=0 (partial mismatch), got %d", result[0].PodCount)
	}
}

// TestHeadlessService 验证 Headless Service (ClusterIP=None) 识别。
func TestHeadlessService(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "headless-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "None",
				Type:      corev1.ServiceTypeClusterIP,
				Selector:  map[string]string{"app": "headless"},
			},
		},
	}
	mock.pods = []corev1.Pod{
		podWithOwner("headless-0", "default", map[string]string{"app": "headless"}, "StatefulSet", "headless"),
	}
	mock.statefulSets = []appsv1.StatefulSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "headless", Namespace: "default"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].ServiceType != "Headless" {
		t.Errorf("expected Headless, got %s", result[0].ServiceType)
	}
}

// TestExternalNameService 验证 ExternalName Service 识别。
func TestExternalNameService(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ext-api", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName, ExternalName: "api.external.com"},
		},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].ServiceType != "ExternalName" {
		t.Errorf("expected ExternalName, got %s", result[0].ServiceType)
	}
	if result[0].WorkloadType != WorkloadTypeUnknown {
		t.Errorf("expected unknown workload for ExternalName, got %s", result[0].WorkloadType)
	}
}

// TestReplicaSetToDeploymentMapping 验证 ReplicaSet 名称前缀 → Deployment 映射。
func TestReplicaSetToDeploymentMapping(t *testing.T) {
	cases := []struct {
		rsName  string
		depName string
	}{
		{"api-7d8f9c6b5", "api"},
		{"my-app-6f9c8d7e4", "my-app"},
		{"single", "single"},
	}
	for _, c := range cases {
		got := deploymentNameFromReplicaSet(c.rsName)
		if got != c.depName {
			t.Errorf("deploymentNameFromReplicaSet(%s) = %s, want %s", c.rsName, got, c.depName)
		}
	}
}

// TestPodCount 验证 PodCount 正确统计。
func TestPodCount(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api"}),
	}
	mock.pods = []corev1.Pod{
		podWithOwner("api-1", "default", map[string]string{"app": "api"}, "ReplicaSet", "api-abc"),
		podWithOwner("api-2", "default", map[string]string{"app": "api"}, "ReplicaSet", "api-abc"),
		podWithOwner("api-3", "default", map[string]string{"app": "api"}, "ReplicaSet", "api-abc"),
		podWithOwner("web-1", "default", map[string]string{"app": "web"}, "ReplicaSet", "web-def"),
	}
	mock.deployments = []appsv1.Deployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}},
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].PodCount != 3 {
		t.Errorf("expected PodCount=3, got %d", result[0].PodCount)
	}
}

// TestFetchedAtNonZero 验证 FetchedAt 非零。
func TestFetchedAtNonZero(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api"}),
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].FetchedAt.IsZero() {
		t.Error("expected FetchedAt non-zero")
	}
}

// TestNoNPlusOneCalls 验证多 Service 不产生 N+1 K8s API 调用。
func TestNoNPlusOneCalls(t *testing.T) {
	mock := newMockK8s()
	// 10 个 Service
	for i := 0; i < 10; i++ {
		name := "svc-" + string(rune('a'+i))
		mock.services = append(mock.services, svcWithSelector(name, "default", map[string]string{"app": name}))
	}
	// 一些 Pod
	for i := 0; i < 5; i++ {
		mock.pods = append(mock.pods, podWithOwner("pod-"+string(rune('a'+i)), "default", map[string]string{"app": "svc-" + string(rune('a'+i))}, "ReplicaSet", "dep-abc"))
	}

	ds := NewDiscoveryService(mock)
	_, err := ds.Discover(context.Background(), "c1", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 每种 API 只能调用 1 次
	for _, method := range []string{"ListServices", "ListDeployments", "ListStatefulSets", "ListDaemonSets", "ListPods"} {
		if mock.callCount[method] != 1 {
			t.Errorf("expected %s called 1 time, got %d (N+1 detected)", method, mock.callCount[method])
		}
	}
}

// TestDifferentClusterIdentity 验证不同 Cluster 的同名 Service 不冲突。
func TestDifferentClusterIdentity(t *testing.T) {
	mock1 := newMockK8s()
	mock1.services = []corev1.Service{svcWithSelector("api", "default", map[string]string{"app": "api"})}
	ds1 := NewDiscoveryService(mock1)
	result1, _ := ds1.Discover(context.Background(), "cluster-a", "default")

	mock2 := newMockK8s()
	mock2.services = []corev1.Service{svcWithSelector("api", "default", map[string]string{"app": "api"})}
	ds2 := NewDiscoveryService(mock2)
	result2, _ := ds2.Discover(context.Background(), "cluster-b", "default")

	if result1[0].Cluster == result2[0].Cluster {
		t.Error("expected different cluster identities")
	}
	if result1[0].Name != result2[0].Name {
		t.Error("expected same service name across clusters")
	}
}

// TestServiceIdentity 验证 Service identity (cluster+namespace+name)。
func TestServiceIdentity(t *testing.T) {
	mock := newMockK8s()
	mock.services = []corev1.Service{
		svcWithSelector("api", "default", map[string]string{"app": "api"}),
	}

	ds := NewDiscoveryService(mock)
	result, err := ds.Discover(context.Background(), "prod", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Cluster != "prod" {
		t.Errorf("expected cluster=prod, got %s", result[0].Cluster)
	}
	if result[0].Namespace != "default" {
		t.Errorf("expected namespace=default, got %s", result[0].Namespace)
	}
	if result[0].Name != "api" {
		t.Errorf("expected name=api, got %s", result[0].Name)
	}
}
