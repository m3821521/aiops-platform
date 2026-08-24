// Package ssrf 提供统一的 SSRF (Server-Side Request Forgery) 防护。
//
// P0-03: 防止 Connection Provider (Jenkins/ArgoCD/Elasticsearch/Grafana/Docker 等)
// 通过用户可控的 Endpoint 访问内部服务或云元数据服务。
//
// 防护策略：
//   - 阻止 loopback 地址 (127.0.0.0/8, ::1)
//   - 阻止 link-local 地址 (169.254.0.0/16, fe80::/10)，包括云元数据服务 169.254.169.254
//   - 阻止 unspecified 地址 (0.0.0.0, ::)
//   - 允许私有地址 (RFC1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)，因为 AIOps 平台需要连接内部中间件
//   - 允许公网地址
//   - 可通过 Allowlist 配置额外允许的地址（用于测试环境）
//   - DNS 解析后验证所有返回的 IP，只要有一个被阻止就拒绝连接
//   - 支持 context cancellation
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"
)

// ErrBlockedAddress 表示连接目标地址被 SSRF 防护阻止。
var ErrBlockedAddress = errors.New("ssrf: blocked address (loopback/link-local/unspecified)")

// SafeTransportConfig 是 SafeTransport 的配置。
type SafeTransportConfig struct {
	// DialTimeout 是 DNS 解析 + TCP 连接的超时时间。
	DialTimeout time.Duration
	// Allowlist 是额外允许的 IP 或 CIDR 列表（用于测试环境）。
	// 即使地址在阻止列表中，只要匹配 Allowlist 就允许连接。
	Allowlist []string
	// BlockPrivate 控制是否阻止 RFC1918 私有地址。
	// 默认 false（允许私有地址，因为 AIOps 需要连接内部中间件）。
	BlockPrivate bool
}

// DefaultConfig 返回默认配置。
// 开发环境（GIN_MODE=debug 或 AIOPS_ENV=dev）自动允许 loopback 地址，
// 方便连接本地 minikube/Prometheus/Elasticsearch 等服务。
// 生产环境保持严格策略，阻止 loopback/link-local/unspecified。
func DefaultConfig() SafeTransportConfig {
	config := SafeTransportConfig{
		DialTimeout:  10 * time.Second,
		Allowlist:    nil,
		BlockPrivate: false,
	}

	// 开发环境自动允许 loopback 地址
	ginMode := os.Getenv("GIN_MODE")
	aiopsEnv := os.Getenv("AIOPS_ENV")
	if ginMode == "debug" || ginMode == "" || aiopsEnv == "dev" || aiopsEnv == "development" {
		config.Allowlist = []string{"127.0.0.1", "::1", "localhost"}
	}

	return config
}

// SafeTransport 是一个 http.RoundTripper，在建立连接前验证目标 IP 地址。
type SafeTransport struct {
	transport *http.Transport
	config    SafeTransportConfig
	allowNets []*net.IPNet
}

// NewSafeTransport 创建一个新的 SafeTransport。
func NewSafeTransport(config SafeTransportConfig) *SafeTransport {
	st := &SafeTransport{
		config: config,
	}
	// 解析 allowlist
	for _, cidr := range config.Allowlist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			st.allowNets = append(st.allowNets, ipNet)
			continue
		}
		// 尝试作为单个 IP 解析
		if ip := net.ParseIP(cidr); ip != nil {
			mask := net.CIDRMask(32, 32)
			if ip.To4() == nil {
				mask = net.CIDRMask(128, 128)
			}
			st.allowNets = append(st.allowNets, &net.IPNet{IP: ip, Mask: mask})
		}
	}

	dialTimeout := config.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
		// 关键：使用自定义 Control 函数在连接前验证 IP
		Control: st.control,
	}

	st.transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return st
}

// RoundTrip 实现 http.RoundTripper 接口。
func (st *SafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return st.transport.RoundTrip(req)
}

// HTTPClient 返回一个使用 SafeTransport 的 http.Client。
// 同时设置 CheckRedirect 阻止重定向到被阻止的地址。
func (st *SafeTransport) HTTPClient() *http.Client {
	return &http.Client{
		Transport:     st,
		Timeout:       30 * time.Second,
		CheckRedirect: st.checkRedirect,
	}
}

// checkRedirect 是 http.Client.CheckRedirect 回调。
// 在跟随重定向前验证重定向目标地址，防止通过 302 重定向绕过 SSRF 防护。
func (st *SafeTransport) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("ssrf: too many redirects (max 10)")
	}
	// 验证重定向目标的主机名
	if err := st.ValidateEndpoint(req.Context(), req.URL.String()); err != nil {
		return fmt.Errorf("ssrf: redirect target blocked: %w", err)
	}
	return nil
}

// control 是 net.Dialer.Control 函数，在 TCP 连接建立前验证目标 IP。
// 注意：Control 函数在 DNS 解析之后、TCP connect 之前调用。
func (st *SafeTransport) control(network, address string, c syscall.RawConn) error {
	// address 格式为 "ip:port"
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf: invalid address %q: %w", address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// 理论上 Control 收到的 address 已经是解析后的 IP，如果不是 IP 则拒绝
		return fmt.Errorf("ssrf: expected IP address, got %q", host)
	}

	return st.validateIP(ip)
}

// validateIP 验证单个 IP 地址是否被允许。
func (st *SafeTransport) validateIP(ip net.IP) error {
	// 检查 allowlist（优先允许）
	for _, ipNet := range st.allowNets {
		if ipNet.Contains(ip) {
			return nil
		}
	}

	// 阻止 loopback
	if ip.IsLoopback() {
		return fmt.Errorf("%w: %s (loopback)", ErrBlockedAddress, ip.String())
	}

	// 阻止 link-local（包括 169.254.169.254 云元数据服务）
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: %s (link-local, may be cloud metadata service)", ErrBlockedAddress, ip.String())
	}

	// 阻止 unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return fmt.Errorf("%w: %s (unspecified)", ErrBlockedAddress, ip.String())
	}

	// 阻止 multicast
	if ip.IsMulticast() {
		return fmt.Errorf("%w: %s (multicast)", ErrBlockedAddress, ip.String())
	}

	// 可选：阻止私有地址
	if st.config.BlockPrivate && ip.IsPrivate() {
		return fmt.Errorf("%w: %s (private)", ErrBlockedAddress, ip.String())
	}

	return nil
}

// ValidateEndpoint 验证一个 endpoint URL 的主机部分是否安全。
// 这会解析 DNS 并检查所有返回的 IP 地址。
// 只要有一个 IP 被阻止，就返回错误。
func (st *SafeTransport) ValidateEndpoint(ctx context.Context, endpoint string) error {
	// 解析 URL
	host := endpoint
	// 如果是完整 URL，提取 host
	if u, err := parseURL(endpoint); err == nil && u.Host != "" {
		host = u.Hostname()
	}

	// 去掉端口
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// 如果已经是 IP，直接验证
	if ip := net.ParseIP(host); ip != nil {
		return st.validateIP(ip)
	}

	// DNS 解析，检查所有返回的 IP
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("ssrf: DNS resolution failed for %q: %w", host, err)
	}

	if len(addrs) == 0 {
		return fmt.Errorf("ssrf: no IP addresses resolved for %q", host)
	}

	for _, addr := range addrs {
		if err := st.validateIP(addr.IP); err != nil {
			return err
		}
	}

	return nil
}

// parseURL 是一个简单的 URL 解析辅助函数，避免循环依赖。
func parseURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}
