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
// 优先使用 Connection-based 配置，其次使用 Legacy config.yaml。
func (f *Factory) BuildPrometheusQuerier(ctx context.Context, rdb *redis.Client) (monitoring.Querier, error) {
	// 1. 尝试从 Connection Manager 获取 prometheus 类型的 Connection
	conns, err := f.connMgr.ListByType(ctx, "prometheus", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Prometheus Connection 失败: %w", err)
	}

	var address string
	var timeout time.Duration

	if len(conns) > 0 {
		conn := conns[0] // 使用第一个 enabled 的 Prometheus Connection
		address = conn.Endpoint
		timeout = 30 * time.Second // 默认超时
	} else if f.legacyCfg != nil {
		// 2. 回退到 Legacy config.yaml
		address = f.legacyCfg.Prometheus.Address
		timeout = f.legacyCfg.Prometheus.Timeout
	}

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
func (f *Factory) BuildElasticsearchClient(ctx context.Context) (*logging.Client, error) {
	// 1. 尝试从 Connection Manager 获取 elasticsearch 类型的 Connection
	conns, err := f.connMgr.ListByType(ctx, "elasticsearch", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Elasticsearch Connection 失败: %w", err)
	}

	var address, index, username, password string
	var timeoutSec int = 30

	if len(conns) > 0 {
		conn := conns[0]
		address = conn.Endpoint
		// 从 Config 中获取 index
		if idx, ok := conn.Config["index"]; ok {
			index = fmt.Sprintf("%v", idx)
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
	} else if f.legacyCfg != nil {
		// 2. 回退到 Legacy config.yaml
		address = f.legacyCfg.Elasticsearch.Address
		index = f.legacyCfg.Elasticsearch.Index
		username = f.legacyCfg.Elasticsearch.Username
		password = f.legacyCfg.Elasticsearch.Password
		timeoutSec = f.legacyCfg.Elasticsearch.Timeout
	}

	if strings.TrimSpace(address) == "" {
		return nil, nil // 未配置时返回 nil，由调用方处理
	}

	return logging.NewClient(address, index, username, password, timeoutSec), nil
}

// BuildJenkinsClient 从 Connection 创建 *automation.JenkinsClient。
// 优先使用 Connection-based 配置，其次使用 Legacy config.yaml。
func (f *Factory) BuildJenkinsClient(ctx context.Context) (*automation.JenkinsClient, error) {
	// 1. 尝试从 Connection Manager 获取 jenkins 类型的 Connection
	conns, err := f.connMgr.ListByType(ctx, "jenkins", true)
	if err != nil {
		return nil, fmt.Errorf("获取 Jenkins Connection 失败: %w", err)
	}

	var baseURL, username, token string
	var timeoutSec int = 30

	if len(conns) > 0 {
		conn := conns[0]
		baseURL = conn.Endpoint
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
	} else if f.legacyCfg != nil {
		// 2. 回退到 Legacy config.yaml
		baseURL = f.legacyCfg.Jenkins.URL
		username = f.legacyCfg.Jenkins.Username
		token = f.legacyCfg.Jenkins.Token
		timeoutSec = f.legacyCfg.Jenkins.Timeout
	}

	if strings.TrimSpace(baseURL) == "" {
		return nil, nil // 未配置时返回 nil
	}

	return automation.NewJenkinsClient(baseURL, username, token, timeoutSec), nil
}

// BuildArgoCDClient 从 Connection 创建 *automation.ArgoCDClient。
// 优先使用 Connection-based 配置，其次使用 Legacy config.yaml。
func (f *Factory) BuildArgoCDClient(ctx context.Context) (*automation.ArgoCDClient, error) {
	// 1. 尝试从 Connection Manager 获取 argocd 类型的 Connection
	conns, err := f.connMgr.ListByType(ctx, "argocd", true)
	if err != nil {
		return nil, fmt.Errorf("获取 ArgoCD Connection 失败: %w", err)
	}

	var baseURL, token string
	var timeoutSec int = 30

	if len(conns) > 0 {
		conn := conns[0]
		baseURL = conn.Endpoint
		// 检查是否为硬编码假地址
		if strings.Contains(baseURL, "argocd.example.com") {
			return nil, nil // 未配置，返回 nil
		}
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
	} else if f.legacyCfg != nil {
		// 2. 回退到 Legacy config.yaml
		baseURL = f.legacyCfg.ArgoCD.URL
		token = f.legacyCfg.ArgoCD.Token
		timeoutSec = f.legacyCfg.ArgoCD.Timeout
		// 检查是否为硬编码假地址
		if strings.Contains(baseURL, "argocd.example.com") {
			return nil, nil
		}
	}

	if strings.TrimSpace(baseURL) == "" {
		return nil, nil // 未配置时返回 nil
	}

	return automation.NewArgoCDClient(baseURL, token, timeoutSec), nil
}
