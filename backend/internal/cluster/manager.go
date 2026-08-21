package cluster

import (
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Manager 管理多个集群的 client-go 客户端。
// 第一次访问某个集群时再创建连接（懒加载），避免启动时所有集群都连不上导致进程起不来。
type Manager struct {
	mu            sync.RWMutex
	clusters      map[string]Cluster
	clients       map[string]kubernetes.Interface
	metricsClient map[string]metricsclientset.Interface
}

func NewManager(clusters []Cluster) *Manager {
	m := &Manager{
		clusters:      make(map[string]Cluster),
		clients:       make(map[string]kubernetes.Interface),
		metricsClient: make(map[string]metricsclientset.Interface),
	}
	for _, c := range clusters {
		m.clusters[c.Name] = c
	}
	return m
}

// SetClusters 运行时更新集群配置（用于 Provider 迁移）。
// 替换现有的集群列表，并清除已缓存的客户端（因为配置可能已改变）。
// 线程安全。
func (m *Manager) SetClusters(clusters []Cluster) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清除旧的集群配置
	m.clusters = make(map[string]Cluster)
	for _, c := range clusters {
		m.clusters[c.Name] = c
	}

	// 清除已缓存的客户端，下次访问时重新创建
	m.clients = make(map[string]kubernetes.Interface)
	m.metricsClient = make(map[string]metricsclientset.Interface)
}

func (m *Manager) List() []Cluster {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Cluster, 0, len(m.clusters))
	for _, c := range m.clusters {
		out = append(out, c)
	}
	return out
}

func (m *Manager) Get(name string) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clusters[name]
	if !ok {
		return Cluster{}, fmt.Errorf("集群不存在: %s", name)
	}
	return c, nil
}

func (m *Manager) Client(name string) (kubernetes.Interface, error) {
	if name == "" {
		list := m.List()
		if len(list) == 0 {
			return nil, fmt.Errorf("没有已启用的 Kubernetes 集群")
		}
		name = list[0].Name
	}

	m.mu.RLock()
	if client, ok := m.clients[name]; ok {
		m.mu.RUnlock()
		return client, nil
	}
	m.mu.RUnlock()

	cluster, err := m.Get(name)
	if err != nil {
		return nil, err
	}
	cfg, err := BuildRESTConfig(cluster)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Kubernetes 客户端失败: %w", err)
	}

	m.mu.Lock()
	m.clients[name] = client
	m.mu.Unlock()
	return client, nil
}

// SetClient 给单元测试注入 fake client。
func (m *Manager) SetClient(name string, client kubernetes.Interface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = client
	if _, ok := m.clusters[name]; !ok {
		m.clusters[name] = Cluster{Name: name, Enabled: true, AuthType: AuthKubeconfig}
	}
}

// RESTConfig 获取指定集群的 rest.Config（用于 exec/port-forward 等需要 SPDY/WebSocket 的操作）。
func (m *Manager) RESTConfig(name string) (*rest.Config, error) {
	if name == "" {
		list := m.List()
		if len(list) == 0 {
			return nil, fmt.Errorf("没有已启用的 Kubernetes 集群")
		}
		name = list[0].Name
	}
	cluster, err := m.Get(name)
	if err != nil {
		return nil, err
	}
	return BuildRESTConfig(cluster)
}

// MetricsClient 获取 metrics clientset（懒加载）。
func (m *Manager) MetricsClient(name string) (metricsclientset.Interface, error) {
	if name == "" {
		list := m.List()
		if len(list) == 0 {
			return nil, fmt.Errorf("没有已启用的 Kubernetes 集群")
		}
		name = list[0].Name
	}

	m.mu.RLock()
	if client, ok := m.metricsClient[name]; ok {
		m.mu.RUnlock()
		return client, nil
	}
	m.mu.RUnlock()

	cluster, err := m.Get(name)
	if err != nil {
		return nil, err
	}
	cfg, err := BuildRESTConfig(cluster)
	if err != nil {
		return nil, err
	}
	client, err := metricsclientset.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Metrics 客户端失败: %w", err)
	}

	m.mu.Lock()
	m.metricsClient[name] = client
	m.mu.Unlock()
	return client, nil
}
