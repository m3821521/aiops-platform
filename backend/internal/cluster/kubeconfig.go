package cluster

import (
	"fmt"
	"os"
	"strings"

	"github.com/aiops/aiops-platform/internal/config"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func BuildRESTConfig(c Cluster) (*rest.Config, error) {
	switch strings.ToLower(strings.TrimSpace(c.AuthType)) {
	case AuthKubeconfig:
		// 优先使用 KubeconfigData（Connection-based），其次使用 KubeconfigPath（Legacy）
		if strings.TrimSpace(c.KubeconfigData) != "" {
			return fromKubeconfigData([]byte(c.KubeconfigData), c.APIServer)
		}
		return fromKubeconfig(c.KubeconfigPath)
	case AuthServiceAccount:
		// 优先使用 TokenData（Connection-based），其次使用 TokenFile（Legacy）
		if strings.TrimSpace(c.TokenData) != "" {
			return fromServiceAccountData(c)
		}
		return fromServiceAccount(c)
	case AuthInCluster:
		return rest.InClusterConfig()
	default:
		return nil, fmt.Errorf("不支持的 auth_type: %s", c.AuthType)
	}
}

// fromKubeconfigData 从 kubeconfig 内容创建 REST Config（Connection-based）。
func fromKubeconfigData(data []byte, apiServer string) (*rest.Config, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("kubeconfig 内容不能为空")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("解析 kubeconfig 失败: %w", err)
	}
	// 如果 Connection 中指定了 APIServer，覆盖 kubeconfig 中的地址
	if strings.TrimSpace(apiServer) != "" {
		cfg.Host = apiServer
	}
	return cfg, nil
}

// fromServiceAccountData 从 token 数据创建 REST Config（Connection-based）。
func fromServiceAccountData(c Cluster) (*rest.Config, error) {
	if strings.TrimSpace(c.APIServer) == "" {
		return nil, fmt.Errorf("serviceaccount 需要 api_server")
	}
	if strings.TrimSpace(c.TokenData) == "" {
		return nil, fmt.Errorf("token 内容不能为空")
	}

	cfg := &rest.Config{
		Host:        c.APIServer,
		BearerToken: strings.TrimSpace(c.TokenData),
	}
	if strings.TrimSpace(c.CAData) != "" {
		cfg.TLSClientConfig = rest.TLSClientConfig{CAData: []byte(c.CAData)}
	} else {
		cfg.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
	}
	return cfg, nil
}

func fromKubeconfig(path string) (*rest.Config, error) {
	path = config.ResolvePath(path)
	if path == "" {
		return nil, fmt.Errorf("kubeconfig_path 不能为空")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("kubeconfig 文件不可用: %w", err)
	}
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}

func fromServiceAccount(c Cluster) (*rest.Config, error) {
	if strings.TrimSpace(c.APIServer) == "" {
		return nil, fmt.Errorf("serviceaccount 需要 api_server")
	}
	token, err := os.ReadFile(config.ResolvePath(c.TokenFile))
	if err != nil {
		return nil, fmt.Errorf("读取 token_file 失败: %w", err)
	}

	cfg := &rest.Config{
		Host:        c.APIServer,
		BearerToken: strings.TrimSpace(string(token)),
	}
	if strings.TrimSpace(c.CAFile) != "" {
		ca, err := os.ReadFile(config.ResolvePath(c.CAFile))
		if err != nil {
			return nil, fmt.Errorf("读取 ca_file 失败: %w", err)
		}
		cfg.TLSClientConfig = rest.TLSClientConfig{CAData: ca}
	} else {
		cfg.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
	}
	return cfg, nil
}
