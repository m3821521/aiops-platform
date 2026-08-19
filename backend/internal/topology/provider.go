package topology

import (
	"context"
	"errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ErrClusterNotFound 集群不存在。
var ErrClusterNotFound = errors.New("cluster not found")

// Provider 是 Kubernetes 拓扑数据提供者接口。
type Provider interface {
	ListNodes(ctx context.Context, cluster string) ([]corev1.Node, error)
	ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error)
	ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error)
	ListServices(ctx context.Context, cluster, namespace string) ([]corev1.Service, error)
	ListIngresses(ctx context.Context, cluster, namespace string) ([]networkingv1.Ingress, error)
}

// K8sProvider 是基于 kubernetes.Interface 的真实实现。
type K8sProvider struct {
	clients map[string]kubernetes.Interface
}

func NewK8sProvider(clients map[string]kubernetes.Interface) *K8sProvider {
	return &K8sProvider{clients: clients}
}

func (p *K8sProvider) client(cluster string) (kubernetes.Interface, error) {
	c, ok := p.clients[cluster]
	if !ok {
		return nil, ErrClusterNotFound
	}
	return c, nil
}

func (p *K8sProvider) ListNodes(ctx context.Context, cluster string) ([]corev1.Node, error) {
	c, err := p.client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *K8sProvider) ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error) {
	c, err := p.client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *K8sProvider) ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error) {
	c, err := p.client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := c.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *K8sProvider) ListServices(ctx context.Context, cluster, namespace string) ([]corev1.Service, error) {
	c, err := p.client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := c.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (p *K8sProvider) ListIngresses(ctx context.Context, cluster, namespace string) ([]networkingv1.Ingress, error) {
	c, err := p.client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := c.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
