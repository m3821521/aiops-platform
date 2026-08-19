package cluster_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aiops/aiops-platform/internal/cluster"
)

func TestLoadRegistryEnabledOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.yaml")
	content := []byte(`
clusters:
  - name: a
    enabled: true
    auth_type: kubeconfig
    kubeconfig_path: /tmp/a
  - name: b
    enabled: false
    auth_type: kubeconfig
    kubeconfig_path: /tmp/b
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	items, err := cluster.LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "a" {
		t.Fatalf("unexpected clusters: %+v", items)
	}
}

func TestBuildRESTConfigServiceAccount(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("demo-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := cluster.BuildRESTConfig(cluster.Cluster{
		AuthType:  cluster.AuthServiceAccount,
		APIServer: "https://127.0.0.1:6443",
		TokenFile: tokenFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://127.0.0.1:6443" {
		t.Fatalf("host=%s", cfg.Host)
	}
	if cfg.BearerToken != "demo-token" {
		t.Fatalf("token=%s", cfg.BearerToken)
	}
}

func TestBuildRESTConfigRejectsUnknownAuth(t *testing.T) {
	_, err := cluster.BuildRESTConfig(cluster.Cluster{AuthType: "password"})
	if err == nil {
		t.Fatal("expected error")
	}
}
