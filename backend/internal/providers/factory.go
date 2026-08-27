package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/connection"
	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/monitoring"
)

// Factory 从 Connection/Credential 创建业务模块需要的 Client。
// 业务代码不再直接读取 config.yaml，而是通过 Factory 获取 Client。
type Factory struct {
	connMgr    *connection.ConnectionManager
	credSvc    *connection.CredentialService
	legacyCfg  *LegacyConfig
}

// LegacyConfig 保存旧 config.yaml 中的配置，用于兼容性回退。
type LegacyConfig struct {
	ClusterConfigPath string
	Prometheus        PrometheusLegacyConfig
	Elasticsearch     ElasticsearchLegacyConfig
	Jenkins           JenkinsLegacyConfig
	ArgoCD            ArgoCDLegacyConfig
}

type PrometheusLegacyConfig struct {
	Address string
	Timeout time.Duration
}

type ElasticsearchLegacyConfig struct {
	Address  string
	Index    string
	Username string
	Password string
	Timeout  int
}

type JenkinsLegacyConfig struct {
	URL      string
	Username string
	Token    string
	Timeout  int
}

type ArgoCDLegacyConfig struct {
	URL     string
	Token   string
	Timeout int
}

// NewFactory 创建 Provider Factory。
func NewFactory(connMgr *connection.ConnectionManager, credSvc *connection.CredentialService, legacy *LegacyConfig) *Factory {
	return &Factory{
		connMgr:   connMgr,
		credSvc:   credSvc,
		legacyCfg: legacy,
	}
}

// BuildKubernetesClusters 从 Connection 创建 []cluster.Cluster。
// 优先使用 Connection-based 配置，其次使用 Legacy config.yaml。
func (f *Factory) BuildKubernetesClusters(ctx context.Context) ([]cluster.Cluster, error) {
	// 1. 尝试从 Connection Manager 获取 kubernetes 类型的 Connection
	conns, err := f.connMgr.ListByType(ctx, "kubernetes", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Kubernetes Connection 失败: %w", err)
	}

	var clusters []cluster.Cluster
	for _, conn := range conns {
		c, err := f.connectionToCluster(ctx, conn)
		if err != nil {
			// 单个 Connection 转换失败不影响其他 Connection
			continue
		}
		clusters = append(clusters, c)
	}

	// 2. 如果没有 Connection-based 配置，回退到 Legacy config.yaml
	if len(clusters) == 0 && f.legacyCfg != nil && f.legacyCfg.ClusterConfigPath != "" {
		legacyClusters, err := cluster.LoadRegistry(f.legacyCfg.ClusterConfigPath)
		if err != nil {
			// Legacy 配置加载失败不报错，返回空列表
			return []cluster.Cluster{}, nil
		}
		return legacyClusters, nil
	}

	return clusters, nil
}

// connectionToCluster 将 Connection 转换为 cluster.Cluster。
func (f *Factory) connectionToCluster(ctx context.Context, conn *connection.Connection) (cluster.Cluster, error) {
	c := cluster.Cluster{
		Name:        conn.Name,
		Enabled:     conn.Enabled,
		Description: conn.Description,
		APIServer:   conn.Endpoint,
	}

	// 获取 Credential 解密数据
	if conn.CredentialID != nil && *conn.CredentialID > 0 {
		credData, err := f.credSvc.GetDecryptedData(ctx, *conn.CredentialID)
		if err != nil {
			return c, fmt.Errorf("获取 Credential 失败: %w", err)
		}

		// 根据 Credential 类型设置认证方式
		if kubeconfig, ok := credData["kubeconfig"]; ok && kubeconfig != "" {
			c.AuthType = cluster.AuthKubeconfig
			// 尝试解码 base64（如果是 base64 编码的）
			if decoded, err := base64.StdEncoding.DecodeString(kubeconfig); err == nil {
				c.KubeconfigData = string(decoded)
			} else {
				c.KubeconfigData = kubeconfig
			}
		} else if token, ok := credData["token"]; ok && token != "" {
			c.AuthType = cluster.AuthServiceAccount
			c.TokenData = token
			if ca, ok := credData["ca"]; ok && ca != "" {
				c.CAData = ca
			}
		} else if username, ok := credData["username"]; ok && username != "" {
			// username/password 转换为 token 方式（Kubernetes 不直接支持 password auth）
			c.AuthType = cluster.AuthServiceAccount
			if password, ok := credData["password"]; ok && password != "" {
				c.TokenData = password // 临时使用 password 作为 token
			}
		}
	}

	// 如果没有 Credential，默认使用 in-cluster 配置
	if c.AuthType == "" {
		c.AuthType = cluster.AuthInCluster
	}

	return c, nil
}

// BuildPrometheusQuerier 从 Connection 创建 monitoring.Querier。
// P2-CONNECTION-DEFAULT-001: 只从 ConnectionManager 获取 enabled Prometheus connection，
// 不再 fallback 到 legacy config.yaml，避免隐式访问 127.0.0.1:9090。
func (f *Factory) BuildPrometheusQuerier(ctx context.Context, rdb *redis.Client) (monitoring.Querier, error) {
	// 1. 从 Connection Manager 获取 enabled 的 prometheus 类型 Connection
	conns, err := f.connMgr.ListByType(ctx, "prometheus", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Prometheus Connection 失败: %w", err)
	}

	if len(conns) == 0 {
		// P2-CONNECTION-DEFAULT-001: 没有配置 Prometheus connection，返回明确错误
		return nil, fmt.Errorf("Prometheus 未配置")
	}

	conn := conns[0] // 使用第一个 enabled 的 Prometheus Connection
	address := conn.Endpoint
	timeout := 30 * time.Second // 默认超时

	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("Prometheus 未配置")
	}

	client, err := monitoring.NewClient(address, timeout)
	if err != nil {
		return nil, fmt.Errorf("创建 Prometheus Client 失败: %w", err)
	}

	if rdb != nil {
		return monitoring.NewCachedQuerier(client, rdb), nil
	}
	return client, nil
}

// BuildElasticsearchClient 从 Connection 创建 *logging.Client。
// 优先使用 Connection-based 配置，其次使用 Legacy config.yaml。
// BuildElasticsearchClient 从 Connection 创建 *logging.Client。
// P2-CONNECTION-DEFAULT-001 Phase 2: 只从 ConnectionManager 获取 enabled Elasticsearch connection，
// 不再 fallback 到 legacy config.yaml，避免隐式访问 127.0.0.1:9200。
func (f *Factory) BuildElasticsearchClient(ctx context.Context) (*logging.Client, error) {
	// 1. 从 Connection Manager 获取 enabled 的 elasticsearch 类型 Connection
	conns, err := f.connMgr.ListByType(ctx, "elasticsearch", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Elasticsearch Connection 失败: %w", err)
	}

	if len(conns) == 0 {
		// P2-CONNECTION-DEFAULT-001: 没有配置 Elasticsearch connection，返回明确错误
		return nil, fmt.Errorf("Elasticsearch 未配置")
	}

	conn := conns[0]
	address := conn.Endpoint
	var index, username, password string
	timeoutSec := 30

	// 从 Config 中获取 index
	if idx, ok := conn.Config["index"]; ok {
		index = fmt.Sprintf("%v", idx)
	}
	if index == "" {
		index = "filebeat-*"
	}
	// 从 Credential 获取 username/password
	if conn.CredentialID != nil && *conn.CredentialID > 0 {
		credData, err := f.credSvc.GetDecryptedData(ctx, *conn.CredentialID)
		if err == nil {
			username, _ = credData["username"]
			password, _ = credData["password"]
			if apiKey, ok := credData["api_key"]; ok && apiKey != "" {
				password = apiKey // 使用 API Key 作为 password
			}
		}
	}

	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("Elasticsearch 未配置")
	}

	return logging.NewClient(address, index, username, password, timeoutSec), nil
}

// BuildJenkinsClient 从 Connection 创建 *automation.JenkinsClient。
// P2-CONNECTION-DEFAULT-001 Phase 2: 只从 ConnectionManager 获取 enabled Jenkins connection，
// 不再 fallback 到 legacy config.yaml，避免隐式访问 127.0.0.1:8080。
func (f *Factory) BuildJenkinsClient(ctx context.Context) (*automation.JenkinsClient, error) {
	// 1. 从 Connection Manager 获取 enabled 的 jenkins 类型 Connection
	conns, err := f.connMgr.ListByType(ctx, "jenkins", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Jenkins Connection 失败: %w", err)
	}

	if len(conns) == 0 {
		// P2-CONNECTION-DEFAULT-001: 没有配置 Jenkins connection，返回明确错误
		return nil, fmt.Errorf("Jenkins 未配置")
	}

	conn := conns[0]
	baseURL := conn.Endpoint
	var username, token string
	timeoutSec := 30

	// 从 Credential 获取 username/token
	if conn.CredentialID != nil && *conn.CredentialID > 0 {
		credData, err := f.credSvc.GetDecryptedData(ctx, *conn.CredentialID)
		if err == nil {
			username, _ = credData["username"]
			token, _ = credData["token"]
			if password, ok := credData["password"]; ok && password != "" && token == "" {
				token = password // 使用 password 作为 token
			}
		}
	}

	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("Jenkins 未配置")
	}

	return automation.NewJenkinsClient(baseURL, username, token, timeoutSec), nil
}

// BuildArgoCDClient 从 Connection 创建 *automation.ArgoCDClient。
// P2-CONNECTION-DEFAULT-001 Phase 2: 只从 ConnectionManager 获取 enabled ArgoCD connection，
// 不再 fallback 到 legacy config.yaml，避免隐式访问默认地址。
func (f *Factory) BuildArgoCDClient(ctx context.Context) (*automation.ArgoCDClient, error) {
	// 1. 从 Connection Manager 获取 enabled 的 argocd 类型 Connection
	conns, err := f.connMgr.ListByType(ctx, "argocd", true)
	if err != nil {
		return nil, fmt.Errorf("获取 ArgoCD Connection 失败: %w", err)
	}

	if len(conns) == 0 {
		// P2-CONNECTION-DEFAULT-001: 没有配置 ArgoCD connection，返回明确错误
		return nil, fmt.Errorf("ArgoCD 未配置")
	}

	conn := conns[0]
	baseURL := conn.Endpoint
	var token string
	timeoutSec := 30

	// 从 Credential 获取 token
	if conn.CredentialID != nil && *conn.CredentialID > 0 {
		credData, err := f.credSvc.GetDecryptedData(ctx, *conn.CredentialID)
		if err == nil {
			token, _ = credData["token"]
			if apiKey, ok := credData["api_key"]; ok && apiKey != "" && token == "" {
				token = apiKey
			}
		}
	}

	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("ArgoCD 未配置")
	}

	return automation.NewArgoCDClient(baseURL, token, timeoutSec), nil
}

// BuildJenkinsClientByID 按 Connection ID 创建 *automation.JenkinsClient。
// 用于 Action 级别指定 Connection 的场景。
func (f *Factory) BuildJenkinsClientByID(ctx context.Context, connectionID int64) (*automation.JenkinsClient, error) {
	conn, err := f.connMgr.GetByID(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("获取 Jenkins Connection 失败: %w", err)
	}
	if conn == nil {
		return nil, fmt.Errorf("jenkins connection not found: %d", connectionID)
	}
	if conn.Type != connection.TypeJenkins {
		return nil, fmt.Errorf("connection type mismatch: expected jenkins, got %s", conn.Type)
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("jenkins connection is disabled: %d", connectionID)
	}

	baseURL := conn.Endpoint
	var username, token string
	timeoutSec := 30

	if conn.CredentialID != nil && *conn.CredentialID > 0 {
		credData, err := f.credSvc.GetDecryptedData(ctx, *conn.CredentialID)
		if err != nil {
			return nil, fmt.Errorf("获取 Jenkins Credential 失败: %w", err)
		}
		username, _ = credData["username"]
		token, _ = credData["token"]
		if password, ok := credData["password"]; ok && password != "" && token == "" {
			token = password
		}
	}

	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("jenkins connection endpoint is empty: %d", connectionID)
	}

	return automation.NewJenkinsClient(baseURL, username, token, timeoutSec), nil
}

// BuildArgoCDClientByID 按 Connection ID 创建 *automation.ArgoCDClient。
// 用于 Action 级别指定 Connection 的场景。
func (f *Factory) BuildArgoCDClientByID(ctx context.Context, connectionID int64) (*automation.ArgoCDClient, error) {
	conn, err := f.connMgr.GetByID(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("获取 ArgoCD Connection 失败: %w", err)
	}
	if conn == nil {
		return nil, fmt.Errorf("argocd connection not found: %d", connectionID)
	}
	if conn.Type != connection.TypeArgoCD {
		return nil, fmt.Errorf("connection type mismatch: expected argocd, got %s", conn.Type)
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("argocd connection is disabled: %d", connectionID)
	}

	baseURL := conn.Endpoint
	var token string
	timeoutSec := 30

	if strings.Contains(baseURL, "argocd.example.com") {
		return nil, fmt.Errorf("argocd connection is not configured (placeholder URL): %d", connectionID)
	}

	if conn.CredentialID != nil && *conn.CredentialID > 0 {
		credData, err := f.credSvc.GetDecryptedData(ctx, *conn.CredentialID)
		if err != nil {
			return nil, fmt.Errorf("获取 ArgoCD Credential 失败: %w", err)
		}
		token, _ = credData["token"]
		if apiKey, ok := credData["api_key"]; ok && apiKey != "" && token == "" {
			token = apiKey
		}
	}

	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("argocd connection endpoint is empty: %d", connectionID)
	}

	return automation.NewArgoCDClient(baseURL, token, timeoutSec), nil
}
