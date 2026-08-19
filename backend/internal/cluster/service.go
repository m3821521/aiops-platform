package cluster

import (
	"context"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type Service struct {
	mgr *Manager
}

func NewService(mgr *Manager) *Service {
	return &Service{mgr: mgr}
}

func (s *Service) Clusters() []Cluster {
	return s.mgr.List()
}

func (s *Service) ListNodes(ctx context.Context, cluster string) ([]corev1.Node, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListNamespaces(ctx context.Context, cluster string) ([]corev1.Namespace, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListStatefulSets(ctx context.Context, cluster, namespace string) ([]appsv1.StatefulSet, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListDaemonSets(ctx context.Context, cluster, namespace string) ([]appsv1.DaemonSet, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListServices(ctx context.Context, cluster, namespace string) ([]corev1.Service, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListConfigMaps(ctx context.Context, cluster, namespace string) ([]corev1.ConfigMap, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Service) ListSecrets(ctx context.Context, cluster, namespace string) ([]corev1.Secret, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func NamespaceOrAll(ns string) string {
	return strings.TrimSpace(ns)
}

// GetPodLogs 获取 Pod 日志（只读操作）。
// tailLines 为返回的最后 N 行，0 表示全部。
func (s *Service) GetPodLogs(ctx context.Context, cluster, namespace, pod, container string, tailLines int64) (string, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return "", err
	}

	opts := &corev1.PodLogOptions{}
	if container != "" {
		opts.Container = container
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}

	stream, err := client.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetPodEvents 获取 Pod 相关的 Event（只读操作）。
func (s *Service) GetPodEvents(ctx context.Context, cluster, namespace, pod string) ([]corev1.Event, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}

	list, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + pod + ",involvedObject.kind=Pod",
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// GetPod 获取单个 Pod 详情
func (s *Service) GetPod(ctx context.Context, cluster, namespace, name string) (*corev1.Pod, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

// GetPodYAML 获取 Pod YAML（脱敏后返回）
func (s *Service) GetPodYAML(ctx context.Context, cluster, namespace, name string) (string, error) {
	pod, err := s.GetPod(ctx, cluster, namespace, name)
	if err != nil {
		return "", err
	}
	// 清理敏感字段
	pod.ManagedFields = nil
	pod.ResourceVersion = ""
	pod.UID = ""
	pod.Generation = 0
	data, err := yaml.Marshal(pod)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetNode 获取单个 Node 详情
func (s *Service) GetNode(ctx context.Context, cluster, name string) (*corev1.Node, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

// GetDeployment 获取单个 Deployment 详情
func (s *Service) GetDeployment(ctx context.Context, cluster, namespace, name string) (*appsv1.Deployment, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

// GetNodeEvents 获取 Node 相关事件
func (s *Service) GetNodeEvents(ctx context.Context, cluster, node string) ([]corev1.Event, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + node + ",involvedObject.kind=Node",
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// RestartPod 重启 Pod（写操作）。
// 通过删除 Pod 让控制器（Deployment/StatefulSet/DaemonSet）自动重建。
func (s *Service) RestartPod(ctx context.Context, cluster, namespace, pod string) error {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return err
	}
	return client.CoreV1().Pods(namespace).Delete(ctx, pod, metav1.DeleteOptions{})
}

// ScaleDeployment 扩容/缩容 Deployment（写操作）。
func (s *Service) ScaleDeployment(ctx context.Context, cluster, namespace, deployment string, replicas int32) error {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return err
	}

	dep, err := client.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return err
	}

	dep.Spec.Replicas = &replicas
	_, err = client.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	return err
}
