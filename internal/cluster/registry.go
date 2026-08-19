package cluster

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/aiops/aiops-platform/internal/config"
)

const (
	AuthKubeconfig     = "kubeconfig"
	AuthServiceAccount = "serviceaccount"
	AuthInCluster      = "incluster"
)

// Cluster 描述一个 Kubernetes 集群怎么连。
// 密钥只保存文件路径，不把 token / kubeconfig 正文写进仓库。
type Cluster struct {
	Name           string `yaml:"name"`
	Enabled        bool   `yaml:"enabled"`
	Description    string `yaml:"description"`
	AuthType       string `yaml:"auth_type"`
	KubeconfigPath string `yaml:"kubeconfig_path"`
	APIServer      string `yaml:"api_server"`
	TokenFile      string `yaml:"token_file"`
	CAFile         string `yaml:"ca_file"`
}

type File struct {
	Clusters []Cluster `yaml:"clusters"`
}

func LoadRegistry(path string) ([]Cluster, error) {
	path = config.ResolvePath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取集群配置失败 (%s): %w", path, err)
	}

	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("解析集群配置失败: %w", err)
	}

	var enabled []Cluster
	for _, item := range file.Clusters {
		if item.Name == "" {
			return nil, fmt.Errorf("集群 name 不能为空")
		}
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	return enabled, nil
}
