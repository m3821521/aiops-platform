package topology

import (
	"context"

	"github.com/aiops/aiops-platform/internal/cluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterManagerProvider 基于 cluster.Manager 的 Provider 实现。
// 按需从 Manager 获取 Kubernetes 客户端（懒加载）。
type ClusterManagerProvider struct {
	mgr *cluster.Manager
}

func NewClusterManagerProvider(mgr *cluster.Manager) *ClusterManagerProvider {
	return &ClusterManagerProvider{mgr: mgr}
}

func (p *ClusterManagerProvider) ListNodes(ctx context.Context, clusterName string) ([]corev1.Node, error) {
	c, err := p.mgr.Client(clusterName)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *ClusterManagerProvider) ListPods(ctx context.Context, clusterName, namespace string) ([]corev1.Pod, error) {
	c, err := p.mgr.Client(clusterName)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *ClusterManagerProvider) ListDeployments(ctx context.Context, clusterName, namespace string) ([]appsv1.Deployment, error) {
	c, err := p.mgr.Client(clusterName)
	if err != nil {
		return nil, err
	}
	list, err := c.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *ClusterManagerProvider) ListServices(ctx context.Context, clusterName, namespace string) ([]corev1.Service, error) {
	c, err := p.mgr.Client(clusterName)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *ClusterManagerProvider) ListIngresses(ctx context.Context, clusterName, namespace string) ([]networkingv1.Ingress, error) {
	c, err := p.mgr.Client(clusterName)
	if err != nil {
		return nil, err
	}
	list, err := c.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
