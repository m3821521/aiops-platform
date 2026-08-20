package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
)

// GrafanaProvider 实现 Grafana 可视化连接 Provider。
//
// 功能：
//   - 从 Connection 获取 endpoint (Grafana URL)
//   - 从 Credential 获取 API Key 或 username/password
//   - Connection Test 调用 /api/health 验证连接
//   - 支持 API Key 认证和 Basic Auth
type GrafanaProvider struct {
	credentialService *connection.CredentialService
	httpClient        *http.Client
}

// NewGrafanaProvider 创建 Grafana Provider。
func NewGrafanaProvider(credentialService *connection.CredentialService) *GrafanaProvider {
	return &GrafanaProvider{
		credentialService: credentialService,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type 返回 Provider 类型。
func (p *GrafanaProvider) Type() connection.ConnectionType {
	return connection.TypeGrafana
}

// Test 测试 Grafana 连接。
// 执行：调用 /api/health → 检查返回状态。
func (p *GrafanaProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	if conn == nil {
		return p.testError(start, "INVALID_CONNECTION", "connection 不能为空"), nil
	}
	if !conn.Enabled {
		return p.testError(start, "CONNECTION_DISABLED", fmt.Sprintf("connection 已禁用: %s", conn.Name)), nil
	}

	if conn.Endpoint == "" {
		return p.testError(start, "EMPTY_ENDPOINT", "Grafana endpoint 不能为空"), nil
	}

	// 构建 health API URL
	baseURL := strings.TrimRight(conn.Endpoint, "/")
	healthURL := fmt.Sprintf("%s/api/health", baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return p.testError(start, "REQUEST_BUILD_FAILED", err.Error()), nil
	}

	// 添加认证
	if err := p.addAuth(ctx, conn, req); err != nil {
		return p.testError(start, "AUTH_FAILED", err.Error()), nil
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "i/o timeout") {
			return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 Grafana: %v", err)), nil
		}
		return p.testError(start, "HTTP_ERROR", fmt.Sprintf("Grafana 请求失败: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return p.testError(start, "AUTHENTICATION_ERROR", "Grafana 认证失败 (401 Unauthorized)"), nil
	}

	if resp.StatusCode == http.StatusForbidden {
		return p.testError(start, "PERMISSION_DENIED", "Grafana 权限不足 (403 Forbidden)"), nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return p.testError(start, "HTTP_ERROR", fmt.Sprintf("Grafana 返回 HTTP %d: %s", resp.StatusCode, string(body))), nil
	}

	// 解析响应
	var health struct {
		Commit   string `json:"commit"`
		Database string `json:"database"`
		Version  string `json:"version"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.testError(start, "READ_BODY_FAILED", fmt.Sprintf("读取 Grafana 响应失败: %v", err)), nil
	}

	if err := json.Unmarshal(body, &health); err != nil {
		return p.testError(start, "PARSE_FAILED", fmt.Sprintf("解析 Grafana 响应失败: %v", err)), nil
	}

	message := "Grafana 连接正常"
	if health.Version != "" {
		message = fmt.Sprintf("Grafana 连接正常, 版本: %s, 数据库: %s", health.Version, health.Database)
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: message,
	}, nil
}

// Connect 创建 Grafana HTTP 客户端配置。
// 返回包含 client 和 baseURL 的结构，业务代码可以直接使用。
func (p *GrafanaProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}

	return &GrafanaClient{
		BaseURL:           strings.TrimRight(conn.Endpoint, "/"),
		HTTPClient:        p.httpClient,
		CredentialService: p.credentialService,
		Connection:        conn,
	}, nil
}

// addAuth 为 HTTP 请求添加认证。
func (p *GrafanaProvider) addAuth(ctx context.Context, conn *connection.Connection, req *http.Request) error {
	if conn.CredentialID == nil {
		return nil
	}

	data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
	if err != nil {
		return fmt.Errorf("解密 credential 失败: %w", err)
	}

	// API Key (优先)
	if apiKey, ok := data["api_key"]; ok && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return nil
	}

	// Token (作为 API Key)
	if token, ok := data["token"]; ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}

	// Basic Auth
	username, hasUser := data["username"]
	password, hasPass := data["password"]
	if hasUser && hasPass {
		req.SetBasicAuth(username, password)
		return nil
	}

	return nil
}

// testError 构造测试失败结果。
func (p *GrafanaProvider) testError(start time.Time, errorCode, errorMessage string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CheckedAt:    time.Now(),
	}
}

// GrafanaClient 是 Grafana HTTP 客户端封装。
type GrafanaClient struct {
	BaseURL           string
	HTTPClient        *http.Client
	CredentialService *connection.CredentialService
	Connection        *connection.Connection
}

// Get 执行 GET 请求。
func (c *GrafanaClient) Get(ctx context.Context, path string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s%s", c.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Grafana 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Grafana 返回 HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// addAuth 为请求添加认证。
func (c *GrafanaClient) addAuth(ctx context.Context, req *http.Request) error {
	if c.Connection.CredentialID == nil {
		return nil
	}

	data, err := c.CredentialService.GetDecryptedData(ctx, *c.Connection.CredentialID)
	if err != nil {
		return err
	}

	if apiKey, ok := data["api_key"]; ok && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return nil
	}

	if token, ok := data["token"]; ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}

	username, hasUser := data["username"]
	password, hasPass := data["password"]
	if hasUser && hasPass {
		req.SetBasicAuth(username, password)
		return nil
	}

	return nil
}
