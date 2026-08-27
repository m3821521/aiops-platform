package connection

import (
	"testing"

	"github.com/aiops/aiops-platform/internal/config"
)

// TestLegacyAdapter_NoElasticsearchWhenAddressEmpty 验证未配置时不创建 elasticsearch-default。
func TestLegacyAdapter_NoElasticsearchWhenAddressEmpty(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{} // Address 为空

	adapter.Load(cfg)

	conns := adapter.List()
	for _, conn := range conns {
		if conn.Type == TypeElasticsearch {
			t.Errorf("Elasticsearch.Address 为空时不应创建 elasticsearch-default， found: %+v", conn)
		}
	}
	if conn := adapter.GetByType(TypeElasticsearch); conn != nil {
		t.Errorf("GetByType(TypeElasticsearch) 应返回 nil")
	}
}

// TestLegacyAdapter_NoJenkinsWhenURLEmpty 验证未配置时不创建 jenkins-default。
func TestLegacyAdapter_NoJenkinsWhenURLEmpty(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{} // URL 为空

	adapter.Load(cfg)

	if conn := adapter.GetByType(TypeJenkins); conn != nil {
		t.Errorf("Jenkins.URL 为空时不应创建 jenkins-default")
	}
}

// TestLegacyAdapter_NoArgoCDWhenURLEmpty 验证未配置时不创建 argocd-default。
func TestLegacyAdapter_NoArgoCDWhenURLEmpty(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{} // URL 为空

	adapter.Load(cfg)

	if conn := adapter.GetByType(TypeArgoCD); conn != nil {
		t.Errorf("ArgoCD.URL 为空时不应创建 argocd-default")
	}
}

// TestLegacyAdapter_ExplicitElasticsearchCreated 验证显式配置时正常创建 legacy connection。
func TestLegacyAdapter_ExplicitElasticsearchCreated(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{
		Elasticsearch: config.Elasticsearch{
			Address: "http://es.example.com:9200",
			Index:   "logs-*",
			Timeout: 30,
		},
	}

	adapter.Load(cfg)

	conn := adapter.GetByType(TypeElasticsearch)
	if conn == nil {
		t.Fatalf("显式配置 Elasticsearch.Address 时应创建 legacy connection")
	}
	if conn.Endpoint != "http://es.example.com:9200" {
		t.Errorf("Endpoint 应为显式配置的地址， got: %q", conn.Endpoint)
	}
	if conn.Name != "elasticsearch-default" {
		t.Errorf("Name 应为 elasticsearch-default， got: %q", conn.Name)
	}
}

// TestLegacyAdapter_ExplicitJenkinsCreated 验证显式配置 Jenkins 时正常创建。
func TestLegacyAdapter_ExplicitJenkinsCreated(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{
		Jenkins: config.Jenkins{
			URL:      "http://jenkins.example.com:8080",
			Username: "admin",
			Timeout:  30,
		},
	}

	adapter.Load(cfg)

	conn := adapter.GetByType(TypeJenkins)
	if conn == nil {
		t.Fatalf("显式配置 Jenkins.URL 时应创建 legacy connection")
	}
	if conn.Endpoint != "http://jenkins.example.com:8080" {
		t.Errorf("Endpoint 应为显式配置的地址， got: %q", conn.Endpoint)
	}
}

// TestLegacyAdapter_ExplicitArgoCDCreated 验证显式配置 ArgoCD 时正常创建。
func TestLegacyAdapter_ExplicitArgoCDCreated(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{
		ArgoCD: config.ArgoCD{
			URL:     "https://argocd.example.com",
			Timeout: 30,
		},
	}

	adapter.Load(cfg)

	conn := adapter.GetByType(TypeArgoCD)
	if conn == nil {
		t.Fatalf("显式配置 ArgoCD.URL 时应创建 legacy connection")
	}
	if conn.Endpoint != "https://argocd.example.com" {
		t.Errorf("Endpoint 应为显式配置的地址， got: %q", conn.Endpoint)
	}
}
