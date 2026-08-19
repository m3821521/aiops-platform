package cluster_test

import (
	"context"
	"testing"

	"github.com/aiops/aiops-platform/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestServiceListResources(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: "default"}, Data: map[string][]byte{"k": []byte("v")}},
	)

	mgr := cluster.NewManager(nil)
	mgr.SetClient("demo", client)
	svc := cluster.NewService(mgr)

	nodes, err := svc.ListNodes(context.Background(), "demo")
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes=%v err=%v", nodes, err)
	}

	pods, err := svc.ListPods(context.Background(), "demo", "default")
	if err != nil || len(pods) != 1 {
		t.Fatalf("pods=%v err=%v", pods, err)
	}

	secrets, err := svc.ListSecrets(context.Background(), "demo", "default")
	if err != nil {
		t.Fatal(err)
	}
	views := cluster.ToSecretViews(secrets)
	if len(views) != 1 || views[0].Keys != 1 {
		t.Fatalf("secret views=%+v", views)
	}
	if string(secrets[0].Data["k"]) == "v" && views[0].Name != "sec1" {
		t.Fatal("view should not expose data fields")
	}
}

func TestManagerMissingCluster(t *testing.T) {
	mgr := cluster.NewManager(nil)
	if _, err := mgr.Client("missing"); err == nil {
		t.Fatal("expected error")
	}
}
