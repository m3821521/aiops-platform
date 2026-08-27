package providers

import (
	"context"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestFactory 创建一个用于测试的 Factory，使用 sqlite 内存数据库。
func newTestFactory(t *testing.T) (*Factory, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&connection.Connection{}); err != nil {
		t.Fatal(err)
	}

	connRepo := connection.NewConnectionRepository(db)
	credRepo := connection.NewCredentialRepository(db)
	credSvc := connection.NewCredentialService(credRepo, nil)
	connSvc := connection.NewConnectionService(connRepo, credSvc)
	legacyAdapter := connection.NewLegacyConfigAdapter()
	// 不调用 legacyAdapter.Load，确保没有 legacy connection
	connMgr := connection.NewConnectionManager(connSvc, legacyAdapter)

	return NewFactory(connMgr, credSvc, nil), db
}

// TestBuildPrometheusQuerier_NoConfig 验证 P2-CONNECTION-DEFAULT-001：
// 没有数据库连接、没有 legacy config 时，返回明确的"Prometheus 未配置"错误。
func TestBuildPrometheusQuerier_NoConfig(t *testing.T) {
	factory, _ := newTestFactory(t)

	querier, err := factory.BuildPrometheusQuerier(context.Background(), nil)

	if err == nil {
		t.Fatal("没有配置 Prometheus 时应该返回错误")
	}
	if querier != nil {
		t.Error("没有配置 Prometheus 时 querier 应为 nil")
	}
	// 验证错误信息明确
	if err.Error() != "Prometheus 未配置" {
		t.Errorf("错误信息应为 'Prometheus 未配置'， got: %q", err.Error())
	}
}

// TestBuildPrometheusQuerier_UsesExplicitConnection 验证有数据库连接时使用连接的 Endpoint。
func TestBuildPrometheusQuerier_UsesExplicitConnection(t *testing.T) {
	factory, db := newTestFactory(t)

	// 创建一个 enabled 的 Prometheus connection
	conn := &connection.Connection{
		Name:        "test-prometheus",
		Type:        connection.TypePrometheus,
		Environment: connection.EnvDev,
		Endpoint:    "http://prometheus-test.example.com:9090",
		Enabled:     true,
		Status:      connection.StatusAvailable,
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatal(err)
	}

	querier, err := factory.BuildPrometheusQuerier(context.Background(), nil)

	if err != nil {
		t.Fatalf("有配置时不应返回错误: %v", err)
	}
	if querier == nil {
		t.Fatal("有配置时 querier 不应为 nil")
	}
}

// TestBuildPrometheusQuerier_DoesNotFallbackToLegacyConfig 验证：
// 即使 Factory 设置了 legacyCfg，ConnectionManager 无连接时也不会 fallback 到 legacy config。
func TestBuildPrometheusQuerier_DoesNotFallbackToLegacyConfig(t *testing.T) {
	factory, _ := newTestFactory(t)

	// 手动设置 legacyCfg（模拟旧配置），但 BuildPrometheusQuerier 不应使用它
	factory.legacyCfg = &LegacyConfig{
		Prometheus: PrometheusLegacyConfig{
			Address: "http://127.0.0.1:9090",
			Timeout: 30 * time.Second,
		},
	}

	querier, err := factory.BuildPrometheusQuerier(context.Background(), nil)

	if err == nil {
		t.Fatal("ConnectionManager 无连接时应返回错误，不应 fallback 到 legacyCfg")
	}
	if querier != nil {
		t.Error("querier 应为 nil")
	}
	if err.Error() != "Prometheus 未配置" {
		t.Errorf("错误信息应为 'Prometheus 未配置'， got: %q", err.Error())
	}
}

// TestBuildPrometheusQuerier_NoConnectionInDB 验证数据库中没有任何 Prometheus 连接时返回错误。
// 注意：ConnectionRepository.ListByType 已在 SQL 层过滤 enabled=true，
// 因此 disabled 的连接不会被返回，这里直接测试空数据库的情况。
func TestBuildPrometheusQuerier_NoConnectionInDB(t *testing.T) {
	factory, _ := newTestFactory(t)

	// 数据库中没有任何 Prometheus 连接
	querier, err := factory.BuildPrometheusQuerier(context.Background(), nil)

	if err == nil {
		t.Fatal("数据库中没有 Prometheus 连接时应返回错误")
	}
	if querier != nil {
		t.Error("querier 应为 nil")
	}
	if err.Error() != "Prometheus 未配置" {
		t.Errorf("错误信息应为 'Prometheus 未配置'， got: %q", err.Error())
	}
}
