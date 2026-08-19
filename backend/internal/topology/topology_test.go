package topology_test

import (
	"context"
	"testing"

	"github.com/aiops/aiops-platform/internal/topology"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeProvider 实现 topology.Provider，用于测试。
type fakeProvider struct {
	nodes       []corev1.Node
	pods        []corev1.Pod
	deployments []appsv1.Deployment
	services    []corev1.Service
	ingresses   []networkingv1.Ingress
	err         error
}

func (f *fakeProvider) ListNodes(ctx context.Context, cluster string) ([]corev1.Node, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.nodes, nil
}
func (f *fakeProvider) ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.pods, nil
}
func (f *fakeProvider) ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.deployments, nil
}
func (f *fakeProvider) ListServices(ctx context.Context, cluster, namespace string) ([]corev1.Service, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.services, nil
}
func (f *fakeProvider) ListIngresses(ctx context.Context, cluster, namespace string) ([]networkingv1.Ingress, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ingresses, nil
}

func makeTestData() *fakeProvider {
	replicas := int32(2)
	return &fakeProvider{
		nodes: []corev1.Node{
			{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		},
		deployments: []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "dep-uid-1"},
				Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
				Status:     appsv1.DeploymentStatus{ReadyReplicas: 2, Replicas: 2},
			},
		},
		pods: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-abc123",
					Namespace: "default",
					Labels:    map[string]string{"app": "web"},
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "web-xyz789"},
					},
				},
				Spec:   corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-def456",
					Namespace: "default",
					Labels:    map[string]string{"app": "web"},
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "web-xyz789"},
					},
				},
				Spec:   corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
		},
		services: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"},
				Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "no-selector-svc", Namespace: "default"},
				Spec:       corev1.ServiceSpec{Selector: nil},
			},
		},
		ingresses: []networkingv1.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "web-ing", Namespace: "default"},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{Name: "web-svc", Port: networkingv1.ServiceBackendPort{Number: 80}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestBuilder_DeploymentPodEdge(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	// 应该有 2 条 owns 边（Deployment → 2 Pods）。
	count := 0
	for _, e := range graph.Edges {
		if e.Relation == topology.RelationOwns {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 owns edges, got %d", count)
	}
}

func TestBuilder_PodNodeEdge(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, e := range graph.Edges {
		if e.Relation == topology.RelationRunsOn {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 runs_on edges, got %d", count)
	}
}

func TestBuilder_ServicePodEdge(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, e := range graph.Edges {
		if e.Relation == topology.RelationSelects {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 selects edges, got %d", count)
	}
}

func TestBuilder_IngressServiceEdge(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, e := range graph.Edges {
		if e.Relation == topology.RelationRoutes {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 routes_to edge, got %d", count)
	}
}

func TestBuilder_NoSelectorService(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	// no-selector-svc 不应该有 selects 边。
	noSelectorID := topology.NodeID("test", topology.TypeService, "default", "no-selector-svc")
	for _, e := range graph.Edges {
		if e.Source == noSelectorID && e.Relation == topology.RelationSelects {
			t.Fatal("no-selector service should not have selects edge")
		}
	}
}

func TestBuilder_DifferentNamespace(t *testing.T) {
	provider := makeTestData()
	// 添加一个不同 namespace 的 Service，selector 匹配但 namespace 不同。
	provider.services = append(provider.services, corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "other-svc", Namespace: "other"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
	})
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	// other-svc 不应该关联到 default namespace 的 Pod。
	otherSvcID := topology.NodeID("test", topology.TypeService, "other", "other-svc")
	for _, e := range graph.Edges {
		if e.Source == otherSvcID {
			t.Fatal("service in different namespace should not select pods")
		}
	}
}

func TestBuilder_StableNodeID(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)

	graph1, _ := builder.Build(context.Background(), "test", "")
	graph2, _ := builder.Build(context.Background(), "test", "")

	if len(graph1.Nodes) != len(graph2.Nodes) {
		t.Fatalf("node count changed: %d vs %d", len(graph1.Nodes), len(graph2.Nodes))
	}

	idSet1 := make(map[string]bool)
	for _, n := range graph1.Nodes {
		idSet1[n.ID] = true
	}
	for _, n := range graph2.Nodes {
		if !idSet1[n.ID] {
			t.Fatalf("node ID not stable: %s", n.ID)
		}
	}
}

func TestBuilder_NoDuplicateEdges(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	graph, err := builder.Build(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}

	edgeSet := make(map[string]bool)
	for _, e := range graph.Edges {
		if edgeSet[e.ID] {
			t.Fatalf("duplicate edge: %s", e.ID)
		}
		edgeSet[e.ID] = true
	}
}

func TestBuilder_K8sAPIError(t *testing.T) {
	provider := &fakeProvider{err: context.DeadlineExceeded}
	builder := topology.NewBuilder(provider)
	_, err := builder.Build(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error when K8s API fails")
	}
}

func TestService_Dependencies(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	svc := topology.NewService(builder, nil, nil)

	result, err := svc.GetDependencies(context.Background(), "test", topology.TypeService, "default", "web-svc")
	if err != nil {
		t.Fatal(err)
	}

	// web-svc 应该有 2 个 children（2 个 Pod）。
	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(result.Children))
	}
}

func TestService_Impact(t *testing.T) {
	provider := makeTestData()
	builder := topology.NewBuilder(provider)
	svc := topology.NewService(builder, nil, nil)

	// Pod 故障应该影响 Deployment 和 Service。
	result, err := svc.GetImpact(context.Background(), "test", topology.TypePod, "default", "web-abc123")
	if err != nil {
		t.Fatal(err)
	}

	// 受影响节点应包含 Deployment 和 Service。
	foundDep := false
	foundSvc := false
	for _, n := range result.AffectedNodes {
		if n.Type == topology.TypeDeployment {
			foundDep = true
		}
		if n.Type == topology.TypeService {
			foundSvc = true
		}
	}
	if !foundDep {
		t.Fatal("expected deployment in affected nodes")
	}
	if !foundSvc {
		t.Fatal("expected service in affected nodes")
	}
}

// 确保 fakeProvider 实现 topology.Provider 接口。
var _ topology.Provider = (*fakeProvider)(nil)
// 确保 fake clientset 被引用（避免未使用导入）。
var _ = fake.NewSimpleClientset
var _ = intstr.FromInt
