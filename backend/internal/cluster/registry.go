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
// 支持通过 Connection/Credential 直接传入数据（KubeconfigData/TokenData/CAData），
// 优先使用 Data 字段，其次使用 Path 字段（向后兼容）。
type Cluster struct {
	Name           string `yaml:"name"`
	Enabled        bool   `yaml:"enabled"`
	Description    string `yaml:"description"`
	AuthType       string `yaml:"auth_type"`
	KubeconfigPath string `yaml:"kubeconfig_path"`
	APIServer      string `yaml:"api_server"`
	TokenFile      string `yaml:"token_file"`
	CAFile         string `yaml:"ca_file"`
	// Connection-based 字段（优先使用）
	KubeconfigData string `yaml:"-"` // kubeconfig 内容（base64 解码后）
	TokenData      string `yaml:"-"` // token 内容
	CAData         string `yaml:"-"` // CA 证书内容
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
