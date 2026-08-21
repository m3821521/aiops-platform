package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
	"github.com/aiops/aiops-platform/internal/ssrf"
)

// PrometheusProvider 实现 connection.MetricsProvider 接口。
//
// 功能：
//   - 从 Connection 获取 endpoint 和 credential
//   - 支持 HTTP Basic Auth 和 Bearer Token
//   - 提供 Prometheus Query/QueryRange 能力
//   - Connection Test 调用 /api/v1/query?query=up
type PrometheusProvider struct {
	credentialService *connection.CredentialService
	httpClient        *http.Client
}

// NewPrometheusProvider 创建 Prometheus Provider。
func NewPrometheusProvider(credentialService *connection.CredentialService) *PrometheusProvider {
	return &PrometheusProvider{
		credentialService: credentialService,
		// P0-03: 使用 SafeTransport 防止 SSRF
		httpClient: ssrf.NewSafeTransport(ssrf.DefaultConfig()).HTTPClient(),
	}
}

// Type 返回 Provider 类型。
func (p *PrometheusProvider) Type() connection.ConnectionType {
	return connection.TypePrometheus
}

// Test 测试 Prometheus 连接。
// 执行：调用 /api/v1/query?query=up → 检查返回 status。
func (p *PrometheusProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	queryURL := fmt.Sprintf("%s/api/v1/query?query=up", conn.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return p.testError(start, "REQUEST_BUILD_FAILED", err.Error()), nil
	}

	// 添加认证
	if err := p.addAuth(ctx, conn, req); err != nil {
		return p.testError(start, "AUTH_FAILED", err.Error()), nil
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 Prometheus: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return p.testError(start, "HTTP_ERROR", fmt.Sprintf("Prometheus 返回 HTTP %d: %s", resp.StatusCode, string(body))), nil
	}

	// 解析响应
	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []interface{} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return p.testError(start, "PARSE_FAILED", fmt.Sprintf("解析 Prometheus 响应失败: %v", err)), nil
	}

	if result.Status != "success" {
		return p.testError(start, "API_ERROR", fmt.Sprintf("Prometheus API 返回 status=%s", result.Status)), nil
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: fmt.Sprintf("Prometheus 正常, up 指标返回 %d 个目标", len(result.Data.Result)),
	}, nil
}

// Connect 创建 Prometheus HTTP 客户端配置。
// 返回 *http.Client 和基础 URL，业务代码可以直接使用。
func (p *PrometheusProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}

	// 返回一个包含 client 和 baseURL 的结构
	return &PrometheusClient{
		BaseURL:           conn.Endpoint,
		HTTPClient:        p.httpClient,
		CredentialService: p.credentialService,
		Connection:        conn,
	}, nil
}

// Query 执行 Prometheus 即时查询。
func (p *PrometheusProvider) Query(ctx context.Context, conn *connection.Connection, query string, timestamp time.Time) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}

	pc, ok := client.(*PrometheusClient)
	if !ok {
		return nil, fmt.Errorf("无效的 Prometheus 客户端")
	}

	return pc.Query(ctx, query, timestamp)
}

// QueryRange 执行 Prometheus 范围查询。
func (p *PrometheusProvider) QueryRange(ctx context.Context, conn *connection.Connection, query string, start, end time.Time, step time.Duration) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}

	pc, ok := client.(*PrometheusClient)
	if !ok {
		return nil, fmt.Errorf("无效的 Prometheus 客户端")
	}

	return pc.QueryRange(ctx, query, start, end, step)
}

// addAuth 为 HTTP 请求添加认证。
func (p *PrometheusProvider) addAuth(ctx context.Context, conn *connection.Connection, req *http.Request) error {
	if conn.CredentialID == nil {
		return nil
	}

	data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
	if err != nil {
		return fmt.Errorf("解密 credential 失败: %w", err)
	}

	// Bearer Token
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

	// API Key
	if apiKey, ok := data["api_key"]; ok && apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
		return nil
	}

	return nil
}

// testError 构造测试失败结果。
func (p *PrometheusProvider) testError(start time.Time, errorCode, errorMessage string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CheckedAt:    time.Now(),
	}
}

// PrometheusClient 是 Prometheus HTTP 客户端封装。
type PrometheusClient struct {
	BaseURL           string
	HTTPClient        *http.Client
	CredentialService *connection.CredentialService
	Connection        *connection.Connection
}

// Query 执行即时查询。
func (c *PrometheusClient) Query(ctx context.Context, query string, timestamp time.Time) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("query", query)
	if !timestamp.IsZero() {
		params.Set("time", timestamp.Format(time.RFC3339))
	}

	reqURL := fmt.Sprintf("%s/api/v1/query?%s", c.BaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Prometheus 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Prometheus 返回 HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// QueryRange 执行范围查询。
func (c *PrometheusClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", start.Format(time.RFC3339))
	params.Set("end", end.Format(time.RFC3339))
	params.Set("step", fmt.Sprintf("%ds", int(step.Seconds())))

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", c.BaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Prometheus 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Prometheus 返回 HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// addAuth 为请求添加认证。
func (c *PrometheusClient) addAuth(ctx context.Context, req *http.Request) error {
	if c.Connection.CredentialID == nil {
		return nil
	}

	data, err := c.CredentialService.GetDecryptedData(ctx, *c.Connection.CredentialID)
	if err != nil {
		return err
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
