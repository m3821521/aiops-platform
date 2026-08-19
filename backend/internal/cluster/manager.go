package cluster

import (
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
)

// Manager 管理多个集群的 client-go 客户端。
// 第一次访问某个集群时再创建连接（懒加载），避免启动时所有集群都连不上导致进程起不来。
type Manager struct {
	mu       sync.RWMutex
	clusters map[string]Cluster
	clients  map[string]kubernetes.Interface
}

func NewManager(clusters []Cluster) *Manager {
	m := &Manager{
		clusters: make(map[string]Cluster),
		clients:  make(map[string]kubernetes.Interface),
	}
	for _, c := range clusters {
		m.clusters[c.Name] = c
	}
	return m
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
