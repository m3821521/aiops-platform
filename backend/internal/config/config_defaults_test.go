package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfig_ElasticsearchNoImplicitDefault 验证 P2-CONNECTION-DEFAULT-001 Phase 2：
// 未配置 elasticsearch.address 时，Address 保持为空，不隐式默认到 127.0.0.1:9200。
func TestConfig_ElasticsearchNoImplicitDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  mode: debug
log:
  level: info
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if cfg.Elasticsearch.Address != "" {
		t.Errorf("未配置 elasticsearch.address 时 Address 应为空， got: %q", cfg.Elasticsearch.Address)
	}

	// Index 默认值可以保留（不是地址）
	if cfg.Elasticsearch.Index == "" {
		t.Log("Elasticsearch.Index 为空（可能需要默认值 filebeat-*）")
	}
}

// TestConfig_JenkinsNoImplicitDefault 验证未配置 jenkins.url 时不隐式默认。
func TestConfig_JenkinsNoImplicitDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  mode: debug
log:
  level: info
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if cfg.Jenkins.URL != "" {
		t.Errorf("未配置 jenkins.url 时 URL 应为空， got: %q", cfg.Jenkins.URL)
	}
}

// TestConfig_ArgoCDNoImplicitDefault 验证未配置 argocd.url 时不隐式默认。
func TestConfig_ArgoCDNoImplicitDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  mode: debug
log:
  level: info
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if cfg.ArgoCD.URL != "" {
		t.Errorf("未配置 argocd.url 时 URL 应为空， got: %q", cfg.ArgoCD.URL)
	}
}

// TestConfig_ExplicitAddressesPreserved 验证显式配置的地址被保留。
func TestConfig_ExplicitAddressesPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  mode: debug
elasticsearch:
  address: http://es.example.com:9200
  index: logs-*
jenkins:
  url: http://jenkins.example.com:8080
  username: admin
argocd:
  url: https://argocd.example.com
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if cfg.Elasticsearch.Address != "http://es.example.com:9200" {
		t.Errorf("显式配置的 Elasticsearch.Address 应被保留， got: %q", cfg.Elasticsearch.Address)
	}
	if cfg.Jenkins.URL != "http://jenkins.example.com:8080" {
		t.Errorf("显式配置的 Jenkins.URL 应被保留， got: %q", cfg.Jenkins.URL)
	}
	if cfg.ArgoCD.URL != "https://argocd.example.com" {
		t.Errorf("显式配置的 ArgoCD.URL 应被保留， got: %q", cfg.ArgoCD.URL)
	}
}
