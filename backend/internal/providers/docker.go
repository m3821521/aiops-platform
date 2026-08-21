package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
)

// DockerProvider 实现 Docker 容器引擎连接 Provider。
//
// 功能：
//   - 从 Connection 获取 endpoint (unix:///var/run/docker.sock 或 tcp://host:2375)
//   - 从 Credential 获取 TLS 证书（如果需要）
//   - Connection Test 调用 /version API 验证连接
//   - 支持 Unix Socket 和 TCP/TLS 连接
//
// 注意：使用标准库 net/http 调用 Docker API，不引入 docker/client 依赖。
type DockerProvider struct {
	credentialService *connection.CredentialService
}

// NewDockerProvider 创建 Docker Provider。
func NewDockerProvider(credentialService *connection.CredentialService) *DockerProvider {
	return &DockerProvider{
		credentialService: credentialService,
	}
}

// Type 返回 Provider 类型。
func (p *DockerProvider) Type() connection.ConnectionType {
	return connection.TypeDocker
}

// Test 测试 Docker 连接。
// 调用：GET /version API，检查 Docker daemon 是否可达。
func (p *DockerProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	if conn == nil {
		return p.testError(start, "INVALID_CONNECTION", "connection 不能为空"), nil
	}
	if !conn.Enabled {
		return p.testError(start, "CONNECTION_DISABLED", fmt.Sprintf("connection 已禁用: %s", conn.Name)), nil
	}

	// 创建 HTTP client
	client, err := p.buildHTTPClient(ctx, conn)
	if err != nil {
		return p.testError(start, "CLIENT_BUILD_FAILED", err.Error()), nil
	}

	// 设置短超时
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 调用 /version API
	req, err := http.NewRequestWithContext(testCtx, "GET", p.getAPIBaseURL(conn)+"/version", nil)
	if err != nil {
		return p.testError(start, "REQUEST_BUILD_FAILED", fmt.Sprintf("创建请求失败: %v", err)), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "no such file") {
			return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 Docker: %v", err)), nil
		}
		if strings.Contains(errMsg, "i/o timeout") || strings.Contains(errMsg, "deadline exceeded") {
			return p.testError(start, "CONNECTION_TIMEOUT", fmt.Sprintf("Docker 连接超时: %v", err)), nil
		}
		if strings.Contains(errMsg, "permission denied") {
			return p.testError(start, "PERMISSION_DENIED", fmt.Sprintf("Docker socket 权限不足: %v", err)), nil
		}
		return p.testError(start, "REQUEST_FAILED", fmt.Sprintf("Docker API 请求失败: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return p.testError(start, "UNEXPECTED_STATUS", fmt.Sprintf("Docker API 返回状态码: %d", resp.StatusCode)), nil
	}

	// 解析版本信息
	var versionInfo struct {
		Version       string `json:"Version"`
		APIVersion    string `json:"ApiVersion"`
		MinAPIVersion string `json:"MinAPIVersion"`
		GitCommit     string `json:"GitCommit"`
		GoVersion     string `json:"GoVersion"`
		Os            string `json:"Os"`
		Arch          string `json:"Arch"`
		KernelVersion string `json:"KernelVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		// 即使解析失败，只要状态码 200 就认为连接成功
		return &connection.TestConnectionResult{
			Status:       connection.StatusAvailable,
			LatencyMs:    time.Since(start).Milliseconds(),
			CheckedAt:    time.Now(),
			ErrorMessage: "Docker 连接正常（版本信息解析失败）",
		}, nil
	}

	message := fmt.Sprintf("Docker 连接正常, 版本: %s, API: %s, OS: %s/%s",
		versionInfo.Version, versionInfo.APIVersion, versionInfo.Os, versionInfo.Arch)

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: message,
	}, nil
}

// Connect 创建 Docker HTTP 客户端。
// 返回 *http.Client，业务代码可以直接使用调用 Docker API。
func (p *DockerProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}

	client, err := p.buildHTTPClient(ctx, conn)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// buildHTTPClient 构建 Docker HTTP 客户端。
// 支持 Unix Socket 和 TCP 连接。
func (p *DockerProvider) buildHTTPClient(ctx context.Context, conn *connection.Connection) (*http.Client, error) {
	if conn.Endpoint == "" {
		return nil, fmt.Errorf("Docker endpoint 不能为空")
	}

	endpoint := conn.Endpoint
	transport := &http.Transport{}

	// Unix Socket 连接
	if strings.HasPrefix(endpoint, "unix://") {
		socketPath := strings.TrimPrefix(endpoint, "unix://")
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		}
	} else if strings.HasPrefix(endpoint, "tcp://") {
		// TCP 连接，移除 tcp:// 前缀
		// HTTP client 会使用标准 TCP 连接
		endpoint = strings.TrimPrefix(endpoint, "tcp://")
	} else if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		// HTTP/HTTPS 连接，保持原样
	} else {
		// 默认当作 TCP 地址
		if !strings.Contains(endpoint, ":") {
			endpoint = endpoint + ":2375"
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	// TODO: TLS 支持（从 Credential 获取 ca/cert/key）
	// 当前阶段先支持 Unix Socket 和无认证 TCP 连接

	return client, nil
}

// getAPIBaseURL 获取 Docker API 基础 URL。
// 对于 Unix Socket，使用 http://docker 作为虚拟 host。
func (p *DockerProvider) getAPIBaseURL(conn *connection.Connection) string {
	endpoint := conn.Endpoint
	if strings.HasPrefix(endpoint, "unix://") {
		return "http://docker"
	}
	if strings.HasPrefix(endpoint, "tcp://") {
		return "http://" + strings.TrimPrefix(endpoint, "tcp://")
	}
	return endpoint
}

// testError 构造测试失败结果。
func (p *DockerProvider) testError(start time.Time, errorCode, errorMessage string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CheckedAt:    time.Now(),
	}
}
