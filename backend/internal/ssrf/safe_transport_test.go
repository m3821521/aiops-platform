package ssrf

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestValidateIP_Loopback(t *testing.T) {
	// 使用严格配置（空 Allowlist），验证 loopback IP 被阻止
	strictConfig := SafeTransportConfig{DialTimeout: 5 * time.Second, Allowlist: nil}
	st := NewSafeTransport(strictConfig)
	err := st.validateIP(net.ParseIP("127.0.0.1"))
	if err == nil {
		t.Error("expected loopback 127.0.0.1 to be blocked")
	}
	err = st.validateIP(net.ParseIP("::1"))
	if err == nil {
		t.Error("expected loopback ::1 to be blocked")
	}
}

func TestValidateIP_LinkLocal(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	// 169.254.169.254 是 AWS/GCP/Azure 云元数据服务地址
	err := st.validateIP(net.ParseIP("169.254.169.254"))
	if err == nil {
		t.Error("expected link-local 169.254.169.254 (cloud metadata) to be blocked")
	}
	err = st.validateIP(net.ParseIP("169.254.1.1"))
	if err == nil {
		t.Error("expected link-local 169.254.1.1 to be blocked")
	}
	err = st.validateIP(net.ParseIP("fe80::1"))
	if err == nil {
		t.Error("expected link-local fe80::1 to be blocked")
	}
}

func TestValidateIP_Unspecified(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	err := st.validateIP(net.ParseIP("0.0.0.0"))
	if err == nil {
		t.Error("expected unspecified 0.0.0.0 to be blocked")
	}
	err = st.validateIP(net.ParseIP("::"))
	if err == nil {
		t.Error("expected unspecified :: to be blocked")
	}
}

func TestValidateIP_PrivateAllowed(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	// 默认允许私有地址（RFC1918），因为 AIOps 需要连接内部中间件
	privateIPs := []string{
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
	}
	for _, ip := range privateIPs {
		if err := st.validateIP(net.ParseIP(ip)); err != nil {
			t.Errorf("expected private IP %s to be allowed by default, got: %v", ip, err)
		}
	}
}

func TestValidateIP_PrivateBlocked(t *testing.T) {
	config := DefaultConfig()
	config.BlockPrivate = true
	st := NewSafeTransport(config)

	privateIPs := []string{
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
	}
	for _, ip := range privateIPs {
		if err := st.validateIP(net.ParseIP(ip)); err == nil {
			t.Errorf("expected private IP %s to be blocked when BlockPrivate=true", ip)
		}
	}
}

func TestValidateIP_PublicAllowed(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"203.0.113.1", // TEST-NET-3，用于文档
	}
	for _, ip := range publicIPs {
		if err := st.validateIP(net.ParseIP(ip)); err != nil {
			t.Errorf("expected public IP %s to be allowed, got: %v", ip, err)
		}
	}
}

func TestAllowlist(t *testing.T) {
	config := DefaultConfig()
	config.Allowlist = []string{"127.0.0.1", "169.254.169.254"}
	st := NewSafeTransport(config)

	// allowlist 中的地址应该被允许
	if err := st.validateIP(net.ParseIP("127.0.0.1")); err != nil {
		t.Errorf("expected allowlisted 127.0.0.1 to be allowed, got: %v", err)
	}
	if err := st.validateIP(net.ParseIP("169.254.169.254")); err != nil {
		t.Errorf("expected allowlisted 169.254.169.254 to be allowed, got: %v", err)
	}
	// 不在 allowlist 中的 loopback 仍然被阻止
	if err := st.validateIP(net.ParseIP("127.0.0.2")); err == nil {
		t.Error("expected non-allowlisted 127.0.0.2 to be blocked")
	}
}

func TestValidateEndpoint_Loopback(t *testing.T) {
	// 使用严格配置（空 Allowlist），验证 loopback 默认被阻止
	strictConfig := SafeTransportConfig{DialTimeout: 5 * time.Second, Allowlist: nil}
	st := NewSafeTransport(strictConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// localhost 解析为 127.0.0.1，应该被阻止
	err := st.ValidateEndpoint(ctx, "http://localhost:8080")
	if err == nil {
		t.Error("expected localhost endpoint to be blocked")
	}
}

func TestValidateEndpoint_CloudMetadata(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 直接使用 IP 地址
	err := st.ValidateEndpoint(ctx, "http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Error("expected cloud metadata endpoint to be blocked")
	}
}

func TestSafeTransport_ImplementsRoundTripper(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	var _ http.RoundTripper = st
	client := st.HTTPClient()
	if client == nil {
		t.Error("expected non-nil http.Client")
	}
	if client.CheckRedirect == nil {
		t.Error("expected CheckRedirect to be set for SSRF protection")
	}
}

// TestCheckRedirect_BlocksLoopback 验证重定向到 loopback 地址被阻止。
func TestCheckRedirect_BlocksLoopback(t *testing.T) {
	// 使用严格配置（空 Allowlist），验证 loopback 重定向被阻止
	strictConfig := SafeTransportConfig{DialTimeout: 5 * time.Second, Allowlist: nil}
	st := NewSafeTransport(strictConfig)
	req, _ := http.NewRequest("GET", "http://127.0.0.1:8080/redirect-target", nil)
	err := st.checkRedirect(req, []*http.Request{})
	if err == nil {
		t.Error("expected redirect to loopback 127.0.0.1 to be blocked")
	}
}

// TestCheckRedirect_BlocksCloudMetadata 验证重定向到云元数据服务被阻止。
func TestCheckRedirect_BlocksCloudMetadata(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	req, _ := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/", nil)
	err := st.checkRedirect(req, []*http.Request{})
	if err == nil {
		t.Error("expected redirect to cloud metadata 169.254.169.254 to be blocked")
	}
}

// TestCheckRedirect_AllowsPublic 验证重定向到公网地址被允许。
func TestCheckRedirect_AllowsPublic(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	// 使用 example.com（公网域名），DNS 解析应该成功
	req, _ := http.NewRequest("GET", "https://example.com/redirect", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	err := st.checkRedirect(req, []*http.Request{})
	if err != nil {
		t.Logf("redirect to example.com blocked (may be DNS issue in test env): %v", err)
		// 不强制失败，因为测试环境可能无法解析公网域名
	}
}

// TestCheckRedirect_TooManyRedirects 验证超过最大重定向次数被阻止。
func TestCheckRedirect_TooManyRedirects(t *testing.T) {
	st := NewSafeTransport(DefaultConfig())
	req, _ := http.NewRequest("GET", "https://example.com/redirect", nil)
	// 模拟已经有 10 次重定向
	via := make([]*http.Request, 10)
	for i := range via {
		via[i], _ = http.NewRequest("GET", "https://example.com/"+string(rune('a'+i)), nil)
	}
	err := st.checkRedirect(req, via)
	if err == nil {
		t.Error("expected too many redirects to be blocked")
	}
}
