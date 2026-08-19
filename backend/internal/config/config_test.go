package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aiops/aiops-platform/internal/config"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`
server:
  host: 127.0.0.1
  port: "9090"
  mode: release
log:
  level: debug
mysql:
  host: 127.0.0.1
  port: 3306
  user: root
  password: secret
  database: aiops
redis:
  address: 127.0.0.1:6379
cluster:
  config_path: configs/clusters.yaml
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG_PATH", path)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != "9090" {
		t.Fatalf("port=%s", cfg.Server.Port)
	}
	if cfg.Server.Addr() != "127.0.0.1:9090" {
		t.Fatalf("addr=%s", cfg.Server.Addr())
	}
}
