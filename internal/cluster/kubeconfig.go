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
		return fromKubeconfig(c.KubeconfigPath)
	case AuthServiceAccount:
		return fromServiceAccount(c)
	case AuthInCluster:
		return rest.InClusterConfig()
	default:
		return nil, fmt.Errorf("不支持的 auth_type: %s", c.AuthType)
	}
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
