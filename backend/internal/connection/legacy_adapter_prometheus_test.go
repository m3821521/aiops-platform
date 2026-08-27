package connection

import (
	"testing"

	"github.com/aiops/aiops-platform/internal/config"
)

// TestLegacyAdapter_NoPrometheusWhenAddressEmpty 验证 P2-CONNECTION-DEFAULT-001：
// 当 config.Prometheus.Address 为空时，不创建 prometheus-default legacy connection。
func TestLegacyAdapter_NoPrometheusWhenAddressEmpty(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	// Config 零值中 Prometheus.Address 为空字符串
	cfg := &config.Config{}

	adapter.Load(cfg)

	conns := adapter.List()
	for _, conn := range conns {
		if conn.Type == TypePrometheus {
			t.Errorf("Prometheus.Address 为空时不应创建 prometheus-default legacy connection， found: %+v", conn)
		}
	}

	// GetByType 也应返回 nil
	if conn := adapter.GetByType(TypePrometheus); conn != nil {
		t.Errorf("GetByType(TypePrometheus) 应返回 nil， got: %+v", conn)
	}
}

// TestLegacyAdapter_PrometheusWhenAddressExplicit 验证显式配置地址时正常创建 legacy connection。
func TestLegacyAdapter_PrometheusWhenAddressExplicit(t *testing.T) {
	adapter := NewLegacyConfigAdapter()
	cfg := &config.Config{
		Prometheus: config.Prometheus{
			Address: "http://prometheus.example.com:9090",
			Timeout: 30,
		},
	}

	adapter.Load(cfg)

	conn := adapter.GetByType(TypePrometheus)
	if conn == nil {
		t.Fatalf("显式配置 Prometheus.Address 时应创建 legacy connection")
	}

	if conn.Endpoint != "http://prometheus.example.com:9090" {
		t.Errorf("Endpoint 应为显式配置的地址， got: %q", conn.Endpoint)
	}

	if conn.Name != "prometheus-default" {
		t.Errorf("Name 应为 prometheus-default， got: %q", conn.Name)
	}

	if !conn.IsSystemDefault {
		t.Errorf("IsSystemDefault 应为 true")
	}
}
