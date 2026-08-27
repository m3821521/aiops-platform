package providers

import (
	"context"
	"testing"

	"github.com/aiops/aiops-platform/internal/connection"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestFactoryForDefaults 创建用于测试的 Factory（sqlite 内存数据库）。
func newTestFactoryForDefaults(t *testing.T) (*Factory, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&connection.Connection{}, &connection.Credential{}); err != nil {
		t.Fatal(err)
	}

	connRepo := connection.NewConnectionRepository(db)
	credRepo := connection.NewCredentialRepository(db)
	credSvc := connection.NewCredentialService(credRepo, nil)
	connSvc := connection.NewConnectionService(connRepo, credSvc)
	legacyAdapter := connection.NewLegacyConfigAdapter()
	connMgr := connection.NewConnectionManager(connSvc, legacyAdapter)

	return NewFactory(connMgr, credSvc, nil), db
}

// TestBuildElasticsearchClient_NoConfig 验证无配置时返回明确错误。
func TestBuildElasticsearchClient_NoConfig(t *testing.T) {
	factory, _ := newTestFactoryForDefaults(t)

	client, err := factory.BuildElasticsearchClient(context.Background())

	if err == nil {
		t.Fatal("无配置时应返回错误")
	}
	if client != nil {
		t.Error("client 应为 nil")
	}
	if err.Error() != "Elasticsearch 未配置" {
		t.Errorf("错误信息应为 'Elasticsearch 未配置'， got: %q", err.Error())
	}
}

// TestBuildElasticsearchClient_UsesExplicitConnection 验证有数据库连接时使用 Endpoint。
func TestBuildElasticsearchClient_UsesExplicitConnection(t *testing.T) {
	factory, db := newTestFactoryForDefaults(t)

	conn := &connection.Connection{
		Name:        "test-es",
		Type:        connection.TypeElasticsearch,
		Environment: connection.EnvDev,
		Endpoint:    "http://es-test.example.com:9200",
		Enabled:     true,
		Status:      connection.StatusAvailable,
		Config:      connection.ConfigMap{"index": "test-*"},
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatal(err)
	}

	client, err := factory.BuildElasticsearchClient(context.Background())

	if err != nil {
		t.Fatalf("有配置时不应返回错误: %v", err)
	}
	if client == nil {
		t.Fatal("client 不应为 nil")
	}
}

// TestBuildElasticsearchClient_NoLegacyFallback 验证 legacyCfg 不再作为 fallback。
func TestBuildElasticsearchClient_NoLegacyFallback(t *testing.T) {
	factory, _ := newTestFactoryForDefaults(t)

	// 设置 legacyCfg，但 BuildElasticsearchClient 不应使用它
	factory.legacyCfg = &LegacyConfig{
		Elasticsearch: ElasticsearchLegacyConfig{
			Address: "http://127.0.0.1:9200",
			Index:   "filebeat-*",
			Timeout: 30,
		},
	}

	client, err := factory.BuildElasticsearchClient(context.Background())

	if err == nil {
		t.Fatal("数据库无连接时应返回错误，不应 fallback 到 legacyCfg")
	}
	if client != nil {
		t.Error("client 应为 nil")
	}
}

// TestBuildJenkinsClient_NoConfig 验证无配置时返回明确错误。
func TestBuildJenkinsClient_NoConfig(t *testing.T) {
	factory, _ := newTestFactoryForDefaults(t)

	client, err := factory.BuildJenkinsClient(context.Background())

	if err == nil {
		t.Fatal("无配置时应返回错误")
	}
	if client != nil {
		t.Error("client 应为 nil")
	}
	if err.Error() != "Jenkins 未配置" {
		t.Errorf("错误信息应为 'Jenkins 未配置'， got: %q", err.Error())
	}
}

// TestBuildJenkinsClient_UsesExplicitConnection 验证有数据库连接时使用 Endpoint。
func TestBuildJenkinsClient_UsesExplicitConnection(t *testing.T) {
	factory, db := newTestFactoryForDefaults(t)

	conn := &connection.Connection{
		Name:        "test-jenkins",
		Type:        connection.TypeJenkins,
		Environment: connection.EnvDev,
		Endpoint:    "http://jenkins-test.example.com:8080",
		Enabled:     true,
		Status:      connection.StatusAvailable,
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatal(err)
	}

	client, err := factory.BuildJenkinsClient(context.Background())

	if err != nil {
		t.Fatalf("有配置时不应返回错误: %v", err)
	}
	if client == nil {
		t.Fatal("client 不应为 nil")
	}
}

// TestBuildArgoCDClient_NoConfig 验证无配置时返回明确错误。
func TestBuildArgoCDClient_NoConfig(t *testing.T) {
	factory, _ := newTestFactoryForDefaults(t)

	client, err := factory.BuildArgoCDClient(context.Background())

	if err == nil {
		t.Fatal("无配置时应返回错误")
	}
	if client != nil {
		t.Error("client 应为 nil")
	}
	if err.Error() != "ArgoCD 未配置" {
		t.Errorf("错误信息应为 'ArgoCD 未配置'， got: %q", err.Error())
	}
}

// TestBuildArgoCDClient_UsesExplicitConnection 验证有数据库连接时使用 Endpoint。
func TestBuildArgoCDClient_UsesExplicitConnection(t *testing.T) {
	factory, db := newTestFactoryForDefaults(t)

	conn := &connection.Connection{
		Name:        "test-argocd",
		Type:        connection.TypeArgoCD,
		Environment: connection.EnvDev,
		Endpoint:    "https://argocd-test.example.com",
		Enabled:     true,
		Status:      connection.StatusAvailable,
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatal(err)
	}

	client, err := factory.BuildArgoCDClient(context.Background())

	if err != nil {
		t.Fatalf("有配置时不应返回错误: %v", err)
	}
	if client == nil {
		t.Fatal("client 不应为 nil")
	}
}

// TestBuildArgoCDClient_NoLegacyFallback 验证 legacyCfg 不再作为 fallback。
func TestBuildArgoCDClient_NoLegacyFallback(t *testing.T) {
	factory, _ := newTestFactoryForDefaults(t)

	factory.legacyCfg = &LegacyConfig{
		ArgoCD: ArgoCDLegacyConfig{
			URL:     "https://argocd.example.com",
			Timeout: 30,
		},
	}

	client, err := factory.BuildArgoCDClient(context.Background())

	if err == nil {
		t.Fatal("数据库无连接时应返回错误，不应 fallback 到 legacyCfg")
	}
	if client != nil {
		t.Error("client 应为 nil")
	}
}
