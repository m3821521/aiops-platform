package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
)

// ArgoCDProvider 实现 connection.CDProvider 接口。
type ArgoCDProvider struct {
	credentialService *connection.CredentialService
	httpClient        *http.Client
}

func NewArgoCDProvider(credentialService *connection.CredentialService) *ArgoCDProvider {
	return &ArgoCDProvider{
		credentialService: credentialService,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *ArgoCDProvider) Type() connection.ConnectionType {
	return connection.TypeArgoCD
}

func (p *ArgoCDProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	// 检查是否是硬编码的假地址
	if conn.Endpoint == "https://argocd.example.com" || conn.Endpoint == "" {
		return &connection.TestConnectionResult{
			Status:       connection.StatusUnavailable,
			LatencyMs:    0,
			ErrorCode:    "NOT_CONFIGURED",
			ErrorMessage: "ArgoCD 未配置，请设置有效的 Endpoint",
			CheckedAt:    time.Now(),
		}, nil
	}

	// ArgoCD API: /api/v1/applications 获取应用列表
	reqURL := fmt.Sprintf("%s/api/v1/applications", conn.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return p.testError(start, "REQUEST_BUILD_FAILED", err.Error()), nil
	}

	if err := p.addAuth(ctx, conn, req); err != nil {
		return p.testError(start, "AUTH_FAILED", err.Error()), nil
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 ArgoCD: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return p.testError(start, "AUTHENTICATION_ERROR", "ArgoCD 认证失败，请检查 Token"), nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return p.testError(start, "NOT_FOUND", "ArgoCD API 返回 404，请检查 URL 配置"), nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return p.testError(start, "HTTP_ERROR", fmt.Sprintf("ArgoCD 返回 HTTP %d: %s", resp.StatusCode, string(body))), nil
	}

	var apps struct {
		Items []interface{} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		return p.testError(start, "PARSE_FAILED", err.Error()), nil
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: fmt.Sprintf("ArgoCD 正常, 应用数量: %d", len(apps.Items)),
	}, nil
}

func (p *ArgoCDProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}
	if conn.Endpoint == "https://argocd.example.com" || conn.Endpoint == "" {
		return nil, fmt.Errorf("ArgoCD 未配置，请设置有效的 Endpoint")
	}
	return &ArgoCDClient{
		BaseURL:           conn.Endpoint,
		HTTPClient:        p.httpClient,
		CredentialService: p.credentialService,
		Connection:        conn,
	}, nil
}

func (p *ArgoCDProvider) GetApplication(ctx context.Context, conn *connection.Connection, name string) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	ac, ok := client.(*ArgoCDClient)
	if !ok {
		return nil, fmt.Errorf("无效的 ArgoCD 客户端")
	}
	return ac.GetApplication(ctx, name)
}

func (p *ArgoCDProvider) Sync(ctx context.Context, conn *connection.Connection, name string) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	ac, ok := client.(*ArgoCDClient)
	if !ok {
		return nil, fmt.Errorf("无效的 ArgoCD 客户端")
	}
	return ac.SyncApplication(ctx, name)
}

func (p *ArgoCDProvider) GetStatus(ctx context.Context, conn *connection.Connection, name string) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	ac, ok := client.(*ArgoCDClient)
	if !ok {
		return nil, fmt.Errorf("无效的 ArgoCD 客户端")
	}
	return ac.GetSyncStatus(ctx, name)
}

func (p *ArgoCDProvider) addAuth(ctx context.Context, conn *connection.Connection, req *http.Request) error {
	if conn.CredentialID == nil {
		return nil
	}
	data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
	if err != nil {
		return err
	}
	if token, ok := data["token"]; ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
	if apiKey, ok := data["api_key"]; ok && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return nil
	}
	return nil
}

func (p *ArgoCDProvider) testError(start time.Time, code, msg string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    code,
		ErrorMessage: msg,
		CheckedAt:    time.Now(),
	}
}

type ArgoCDClient struct {
	BaseURL           string
	HTTPClient        *http.Client
	CredentialService *connection.CredentialService
	Connection        *connection.Connection
}

func (c *ArgoCDClient) GetApplication(ctx context.Context, name string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/applications/%s", c.BaseURL, name)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ArgoCD 获取应用失败: HTTP %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ArgoCDClient) SyncApplication(ctx context.Context, name string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/api/v1/applications/%s/sync", c.BaseURL, name)
	body := map[string]interface{}{
		"prune":  false,
		"dryRun": false,
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ArgoCD 同步失败: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ArgoCDClient) GetSyncStatus(ctx context.Context, name string) (map[string]interface{}, error) {
	app, err := c.GetApplication(ctx, name)
	if err != nil {
		return nil, err
	}
	status := map[string]interface{}{
		"sync_status":   "Unknown",
		"health_status": "Unknown",
	}
	if statusMap, ok := app["status"].(map[string]interface{}); ok {
		if sync, ok := statusMap["sync"].(map[string]interface{}); ok {
			if s, ok := sync["status"].(string); ok {
				status["sync_status"] = s
			}
		}
		if health, ok := statusMap["health"].(map[string]interface{}); ok {
			if s, ok := health["status"].(string); ok {
				status["health_status"] = s
			}
		}
	}
	return status, nil
}

func (c *ArgoCDClient) addAuth(ctx context.Context, req *http.Request) error {
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
	return nil
}
