package connection

import (
	"context"
	"sync"
	"time"

	"github.com/aiops/aiops-platform/internal/config"
)

// LegacyConfigAdapter 是旧 config.yaml 配置的兼容性适配器。
//
// 设计目标：
//   - 将旧的 config.yaml 中的外部系统配置映射为 Connection 对象
//   - 保持 P4 兼容，不修改现有业务代码
//   - 内存中映射，不写入数据库（避免重复创建）
//   - 幂等：多次调用不会产生重复 Connection
//
// 约束：
//   - 不自动修改 config.yaml
//   - 不删除旧配置机制
//   - 标记为 system_default，不允许删除和修改类型
//   - 逐步迁移，确认稳定后再逐个迁移到数据库
type LegacyConfigAdapter struct {
	mu          sync.RWMutex
	connections map[string]*Connection // key: type-name
	loaded      bool
}

// NewLegacyConfigAdapter 创建 Legacy Config Adapter。
func NewLegacyConfigAdapter() *LegacyConfigAdapter {
	return &LegacyConfigAdapter{
		connections: make(map[string]*Connection),
	}
}

// Load 从 config.Config 加载旧配置并映射为 Connection。
// 此方法是幂等的，多次调用不会产生重复 Connection。
func (a *LegacyConfigAdapter) Load(cfg *config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.loaded {
		return
	}

	now := time.Now()

	// Prometheus
	if cfg.Prometheus.Address != "" {
		key := string(TypePrometheus) + "-default"
		a.connections[key] = &Connection{
			ID:              -1, // 负 ID 表示内存中的 legacy connection
			Name:            "prometheus-default",
			Type:            TypePrometheus,
			Environment:     EnvDev,
			Endpoint:        cfg.Prometheus.Address,
			Config:          ConfigMap{"timeout": cfg.Prometheus.Timeout},
			Enabled:         true,
			Status:          StatusUnknown,
			Description:     "Legacy Prometheus configuration from config.yaml",
			IsSystemDefault: true,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	// Elasticsearch
	if cfg.Elasticsearch.Address != "" {
		key := string(TypeElasticsearch) + "-default"
		a.connections[key] = &Connection{
			ID:              -2,
			Name:            "elasticsearch-default",
			Type:            TypeElasticsearch,
			Environment:     EnvDev,
			Endpoint:        cfg.Elasticsearch.Address,
			Config:          ConfigMap{"index": cfg.Elasticsearch.Index, "timeout": cfg.Elasticsearch.Timeout},
			Enabled:         true,
			Status:          StatusUnknown,
			Description:     "Legacy Elasticsearch configuration from config.yaml",
			IsSystemDefault: true,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	// Jenkins
	if cfg.Jenkins.URL != "" {
		key := string(TypeJenkins) + "-default"
		a.connections[key] = &Connection{
			ID:              -3,
			Name:            "jenkins-default",
			Type:            TypeJenkins,
			Environment:     EnvDev,
			Endpoint:        cfg.Jenkins.URL,
			Config:          ConfigMap{"username": cfg.Jenkins.Username, "timeout": cfg.Jenkins.Timeout},
			Enabled:         true,
			Status:          StatusUnknown,
			Description:     "Legacy Jenkins configuration from config.yaml",
			IsSystemDefault: true,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	// ArgoCD
	if cfg.ArgoCD.URL != "" {
		key := string(TypeArgoCD) + "-default"
		a.connections[key] = &Connection{
			ID:              -4,
			Name:            "argocd-default",
			Type:            TypeArgoCD,
			Environment:     EnvDev,
			Endpoint:        cfg.ArgoCD.URL,
			Config:          ConfigMap{"timeout": cfg.ArgoCD.Timeout},
			Enabled:         true,
			Status:          StatusUnknown,
			Description:     "Legacy ArgoCD configuration from config.yaml",
			IsSystemDefault: true,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	// MySQL (平台自身数据库，通常不作为外部 Connection)
	// Redis (平台自身缓存，通常不作为外部 Connection)

	a.loaded = true
}

// List 返回所有从旧配置加载的 Connection。
func (a *LegacyConfigAdapter) List() []*Connection {
	a.mu.RLock()
	defer a.mu.RUnlock()

	conns := make([]*Connection, 0, len(a.connections))
	for _, conn := range a.connections {
		conns = append(conns, conn)
	}
	return conns
}

// GetByType 根据类型获取旧配置的 Connection。
// 如果同类型有多个，返回第一个。
func (a *LegacyConfigAdapter) GetByType(connType ConnectionType) *Connection {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, conn := range a.connections {
		if conn.Type == connType {
			return conn
		}
	}
	return nil
}

// GetByName 根据名称获取旧配置的 Connection。
func (a *LegacyConfigAdapter) GetByName(name string) *Connection {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, conn := range a.connections {
		if conn.Name == name {
			return conn
		}
	}
	return nil
}

// IsLoaded 检查是否已加载旧配置。
func (a *LegacyConfigAdapter) IsLoaded() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.loaded
}

// Count 返回旧配置 Connection 的数量。
func (a *LegacyConfigAdapter) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.connections)
}

// ConnectionManager 是统一的 Connection 管理器，整合数据库 Connection 和 Legacy Config。
//
// 功能：
//   - 查询时同时返回数据库 Connection 和 Legacy Config Connection
//   - Legacy Config Connection 标记为 system_default
//   - 保持 P4 兼容
type ConnectionManager struct {
	service       *ConnectionService
	legacyAdapter *LegacyConfigAdapter
}

// NewConnectionManager 创建 Connection Manager。
func NewConnectionManager(service *ConnectionService, legacyAdapter *LegacyConfigAdapter) *ConnectionManager {
	return &ConnectionManager{
		service:       service,
		legacyAdapter: legacyAdapter,
	}
}

// ListAll 返回所有 Connection（数据库 + Legacy Config）。
// Legacy Config Connection 会被过滤掉，如果数据库中已有同名 Connection。
func (m *ConnectionManager) ListAll(ctx context.Context, filter ConnectionFilter, page, pageSize int) ([]ConnectionView, int64, error) {
	// 获取数据库中的 Connection
	dbViews, total, err := m.service.List(ctx, filter, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	// 获取 Legacy Config Connection
	legacyConns := m.legacyAdapter.List()

	// 过滤掉数据库中已存在的 Legacy Connection（按名称去重）
	existingNames := make(map[string]bool)
	for _, view := range dbViews {
		existingNames[view.Name] = true
	}

	var legacyViews []ConnectionView
	for _, conn := range legacyConns {
		if !existingNames[conn.Name] {
			// 检查是否符合过滤条件
			if filter.Type != "" && conn.Type != filter.Type {
				continue
			}
			if filter.Environment != "" && conn.Environment != filter.Environment {
				continue
			}
			if filter.Enabled != nil && conn.Enabled != *filter.Enabled {
				continue
			}
			legacyViews = append(legacyViews, *m.service.toView(ctx, conn))
		}
	}

	// 合并结果
	allViews := append(dbViews, legacyViews...)
	totalWithLegacy := total + int64(len(legacyViews))

	return allViews, totalWithLegacy, nil
}

// GetByType 返回指定类型的第一个可用 Connection。
// 优先返回数据库中的 Connection，如果没有则返回 Legacy Config。
func (m *ConnectionManager) GetByType(ctx context.Context, connType ConnectionType) (*Connection, error) {
	// 优先从数据库获取
	dbConns, err := m.service.ListByType(ctx, connType)
	if err == nil && len(dbConns) > 0 {
		return &dbConns[0], nil
	}

	// 从 Legacy Config 获取
	legacyConn := m.legacyAdapter.GetByType(connType)
	if legacyConn != nil {
		return legacyConn, nil
	}

	return nil, nil
}

// ListByType 返回指定类型的所有启用 Connection（数据库 + Legacy Config）。
// 用于 Provider Factory 创建业务 Client。
func (m *ConnectionManager) ListByType(ctx context.Context, connType string, enabledOnly bool) ([]*Connection, error) {
	var result []*Connection

	// 1. 从数据库获取
	dbConns, err := m.service.ListByType(ctx, ConnectionType(connType))
	if err == nil {
		for i := range dbConns {
			if enabledOnly && !dbConns[i].Enabled {
				continue
			}
			result = append(result, &dbConns[i])
		}
	}

	// 2. 从 Legacy Config 获取（去重）
	existingNames := make(map[string]bool)
	for _, conn := range result {
		existingNames[conn.Name] = true
	}

	legacyConns := m.legacyAdapter.List()
	for i := range legacyConns {
		if legacyConns[i].Type != ConnectionType(connType) {
			continue
		}
		if enabledOnly && !legacyConns[i].Enabled {
			continue
		}
		if existingNames[legacyConns[i].Name] {
			continue
		}
		result = append(result, legacyConns[i])
	}

	return result, nil
}
