package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 是整个平台的配置。
// 对应 Java 里常见的 application.yml。
type Config struct {
	Server        Server        `yaml:"server"`
	Log           Log           `yaml:"log"`
	Mysql         Mysql         `yaml:"mysql"`
	Redis         Redis         `yaml:"redis"`
	Cluster       Cluster       `yaml:"cluster"`
	Prometheus    Prometheus    `yaml:"prometheus"`
	Elasticsearch Elasticsearch `yaml:"elasticsearch"`
	AI            AI            `yaml:"ai"`
	Jenkins       Jenkins       `yaml:"jenkins"`
	ArgoCD        ArgoCD        `yaml:"argocd"`
	Auth          Auth          `yaml:"auth"`
}

type Server struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
	Mode string `yaml:"mode"`
}

type Log struct {
	Level string `yaml:"level"`
}

type Mysql struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	Charset  string `yaml:"charset"`
	MaxIdle  int    `yaml:"max_idle"`
	MaxOpen  int    `yaml:"max_open"`
}

type Redis struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type Cluster struct {
	ConfigPath string `yaml:"config_path"`
}

type Prometheus struct {
	Address string `yaml:"address"`
	Timeout int    `yaml:"timeout"` // 秒
}

type Elasticsearch struct {
	Address  string `yaml:"address"`
	Index    string `yaml:"index"` // 日志索引，如 filebeat-*, logstash-*
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Timeout  int    `yaml:"timeout"` // 秒
}

type AI struct {
	Provider string `yaml:"provider"` // openai / azure / ollama / custom
	BaseURL  string `yaml:"base_url"` // API 地址
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`   // 模型名称
	Timeout  int    `yaml:"timeout"` // 秒
	Enabled  bool   `yaml:"enabled"`
}

type Jenkins struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Token    string `yaml:"token"`   // API Token，不是密码
	Timeout  int    `yaml:"timeout"` // 秒
}

type ArgoCD struct {
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`   // API Token
	Timeout int    `yaml:"timeout"` // 秒
}

type Auth struct {
	JWTSecret     string `yaml:"jwt_secret"`     // JWT 签名密钥
	JWTExpiration int    `yaml:"jwt_expiration"` // Token 有效期（小时）
	Enabled       bool   `yaml:"enabled"`        // 是否启用认证（开发环境可关闭）
}

func (m Mysql) DSN() string {
	charset := m.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database, charset)
}

func (s Server) Addr() string {
	host := s.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := s.Port
	if port == "" {
		port = "8080"
	}
	return host + ":" + port
}

// Load 读取 YAML 配置。
// 查找顺序：环境变量 CONFIG_PATH -> 传入 path -> configs/config.yaml
func Load(path string) (*Config, error) {
	if env := strings.TrimSpace(os.Getenv("CONFIG_PATH")); env != "" {
		path = env
	}
	if strings.TrimSpace(path) == "" {
		path = "configs/config.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败 (%s): %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "debug"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Cluster.ConfigPath == "" {
		cfg.Cluster.ConfigPath = "configs/clusters.yaml"
	}
	if cfg.Prometheus.Address == "" {
		cfg.Prometheus.Address = "http://127.0.0.1:9090"
	}
	if cfg.Prometheus.Timeout <= 0 {
		cfg.Prometheus.Timeout = 30
	}
	if cfg.Elasticsearch.Address == "" {
		cfg.Elasticsearch.Address = "http://127.0.0.1:9200"
	}
	if cfg.Elasticsearch.Index == "" {
		cfg.Elasticsearch.Index = "filebeat-*"
	}
	if cfg.Elasticsearch.Timeout <= 0 {
		cfg.Elasticsearch.Timeout = 30
	}
	if cfg.AI.Provider == "" {
		cfg.AI.Provider = "openai"
	}
	if cfg.AI.BaseURL == "" {
		cfg.AI.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.AI.Model == "" {
		cfg.AI.Model = "gpt-4o-mini"
	}
	if cfg.AI.Timeout <= 0 {
		cfg.AI.Timeout = 60
	}
	if cfg.Jenkins.URL == "" {
		cfg.Jenkins.URL = "http://127.0.0.1:8080"
	}
	if cfg.Jenkins.Timeout <= 0 {
		cfg.Jenkins.Timeout = 30
	}
	if cfg.ArgoCD.URL == "" {
		cfg.ArgoCD.URL = "https://argocd.example.com"
	}
	if cfg.ArgoCD.Timeout <= 0 {
		cfg.ArgoCD.Timeout = 30
	}
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = "aiops-default-secret-change-in-production"
	}
	if cfg.Auth.JWTExpiration <= 0 {
		cfg.Auth.JWTExpiration = 24
	}
	return cfg, nil
}

func ResolvePath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}
