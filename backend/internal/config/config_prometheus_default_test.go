package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfig_PrometheusAddressNoImplicitDefault 验证 P2-CONNECTION-DEFAULT-001：
// 配置文件中未设置 prometheus.address 时，Load 后 Address 保持为空字符串，
// 不隐式默认到 127.0.0.1:9090。
func TestConfig_PrometheusAddressNoImplicitDefault(t *testing.T) {
	// 创建一个没有 prometheus.address 的临时配置文件
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

	if cfg.Prometheus.Address != "" {
		t.Errorf("未配置 prometheus.address 时 Address 应为空， got: %q", cfg.Prometheus.Address)
	}

	// 验证 timeout 默认值仍然存在（与本 Phase 无关，确保没有误删）
	if cfg.Prometheus.Timeout <= 0 {
		t.Errorf("Prometheus.Timeout 应该有默认值， got: %d", cfg.Prometheus.Timeout)
	}
}

// TestConfig_PrometheusExplicitAddressPreserved 验证显式配置的地址被保留。
func TestConfig_PrometheusExplicitAddressPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  mode: debug
prometheus:
  address: http://prometheus.example.com:9090
  timeout: 60
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if cfg.Prometheus.Address != "http://prometheus.example.com:9090" {
		t.Errorf("显式配置的 Prometheus.Address 应该被保留， got: %q", cfg.Prometheus.Address)
	}

	if cfg.Prometheus.Timeout != 60 {
		t.Errorf("显式配置的 Prometheus.Timeout 应该被保留， got: %d", cfg.Prometheus.Timeout)
	}
}
