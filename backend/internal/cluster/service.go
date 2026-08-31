package cluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
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

// GetService 获取单个 Service 详情（只读操作）。
func (s *Service) GetService(ctx context.Context, cluster, namespace, name string) (*corev1.Service, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	svc, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// GetEndpoints 获取 Service 对应的 Endpoints（只读操作）。
func (s *Service) GetEndpoints(ctx context.Context, cluster, namespace, name string) (*corev1.Endpoints, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	ep, err := client.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return ep, nil
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
// timestamps 为 true 时由 Kubernetes API 在每行日志前添加 RFC3339 时间戳。
func (s *Service) GetPodLogs(ctx context.Context, cluster, namespace, pod, container string, tailLines int64, timestamps bool) (string, error) {
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
	if timestamps {
		opts.Timestamps = true
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

// ExecResult 是 Pod 命令执行结果。
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// ExecPod 在 Pod 中执行命令（一次性执行，非交互式终端）。
// command 为命令数组，例如 ["sh", "-c", "ls -la"]。
func (s *Service) ExecPod(ctx context.Context, cluster, namespace, pod, container string, command []string) (*ExecResult, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	restCfg, err := s.mgr.RESTConfig(cluster)
	if err != nil {
		return nil, err
	}

	execOpts := &corev1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(execOpts, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restCfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("创建 executor 失败: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		result.ExitCode = 1
	}
	return result, nil
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

// NodeMetric 节点指标。
type NodeMetric struct {
	Name          string  `json:"name"`
	CPUCores      string  `json:"cpu_cores"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryBytes   string  `json:"memory_bytes"`
	MemoryPercent float64 `json:"memory_percent"`
	Window        string  `json:"window"`
	// Timestamp 是 Metrics Server 实际采集时间（来自 metrics.k8s.io API），不是 Backend fetch 时间。
	Timestamp string `json:"timestamp"`
}

// PodMetric Pod 指标。
type PodMetric struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	CPUCores    string `json:"cpu_cores"`
	MemoryBytes string `json:"memory_bytes"`
	Window      string `json:"window"`
	// Timestamp 是 Metrics Server 实际采集时间（来自 metrics.k8s.io API），不是 Backend fetch 时间。
	Timestamp string `json:"timestamp"`
}

// GetNodeMetrics 获取所有节点的 CPU 和内存使用率（通过 metrics-server）。
func (s *Service) GetNodeMetrics(ctx context.Context, cluster string) ([]NodeMetric, error) {
	client, err := s.mgr.Client(cluster)
	if err != nil {
		return nil, err
	}
	metricsClient, err := s.mgr.MetricsClient(cluster)
	if err != nil {
		return nil, err
	}

	// 获取节点列表，用于计算 CPU/内存百分比
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodeCapacity := make(map[string]struct {
		cpu    int64
		memory int64
	})
	for _, node := range nodes.Items {
		cpu := node.Status.Capacity.Cpu().MilliValue()
		memory := node.Status.Capacity.Memory().Value()
		nodeCapacity[node.Name] = struct {
			cpu    int64
			memory int64
		}{cpu: cpu, memory: memory}
	}

	// 获取节点指标
	metrics, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var result []NodeMetric
	for _, m := range metrics.Items {
		cpuMilli := m.Usage.Cpu().MilliValue()
		memoryBytes := m.Usage.Memory().Value()
		capacity, ok := nodeCapacity[m.Name]
		var cpuPercent, memoryPercent float64
		if ok && capacity.cpu > 0 {
			cpuPercent = float64(cpuMilli) / float64(capacity.cpu) * 100
		}
		if ok && capacity.memory > 0 {
			memoryPercent = float64(memoryBytes) / float64(capacity.memory) * 100
		}
		result = append(result, NodeMetric{
			Name:          m.Name,
			CPUCores:      m.Usage.Cpu().String(),
			CPUPercent:    cpuPercent,
			MemoryBytes:   m.Usage.Memory().String(),
			MemoryPercent: memoryPercent,
			Window:        m.Window.Duration.String(),
			Timestamp:     m.Timestamp.UTC().Format(time.RFC3339),
		})
	}
	return result, nil
}

// GetPodMetrics 获取 Pod 的 CPU 和内存使用量（通过 metrics-server）。
// namespace 为空时查询所有命名空间。
func (s *Service) GetPodMetrics(ctx context.Context, cluster, namespace string) ([]PodMetric, error) {
	metricsClient, err := s.mgr.MetricsClient(cluster)
	if err != nil {
		return nil, err
	}

	var metrics *metricsv1beta1.PodMetricsList
	if namespace == "" {
		metrics, err = metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	} else {
		metrics, err = metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	var result []PodMetric
	for _, m := range metrics.Items {
		// 聚合所有容器的 CPU 和内存
		var cpuTotal, memoryTotal int64
		for _, container := range m.Containers {
			cpuTotal += container.Usage.Cpu().MilliValue()
			memoryTotal += container.Usage.Memory().Value()
		}
		// 转换为字符串
		cpuStr := fmt.Sprintf("%dm", cpuTotal)
		memoryStr := fmt.Sprintf("%dKi", memoryTotal/1024)
		result = append(result, PodMetric{
			Namespace:   m.Namespace,
			Name:        m.Name,
			CPUCores:    cpuStr,
			MemoryBytes: memoryStr,
			Window:      m.Window.Duration.String(),
			Timestamp:   m.Timestamp.UTC().Format(time.RFC3339),
		})
	}
	return result, nil
}
