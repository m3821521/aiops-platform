package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
)

// JenkinsProvider 实现 connection.CIProvider 接口。
type JenkinsProvider struct {
	credentialService *connection.CredentialService
	httpClient        *http.Client
}

func NewJenkinsProvider(credentialService *connection.CredentialService) *JenkinsProvider {
	return &JenkinsProvider{
		credentialService: credentialService,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *JenkinsProvider) Type() connection.ConnectionType {
	return connection.TypeJenkins
}

func (p *JenkinsProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	// Jenkins API: /api/json 获取 Jenkins 信息
	reqURL := fmt.Sprintf("%s/api/json", conn.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return p.testError(start, "REQUEST_BUILD_FAILED", err.Error()), nil
	}

	if err := p.addAuth(ctx, conn, req); err != nil {
		return p.testError(start, "AUTH_FAILED", err.Error()), nil
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 Jenkins: %v", err)), nil
	}
	defer resp.Body.Close()

	// Jenkins 未配置或路径错误时返回 404
	if resp.StatusCode == http.StatusNotFound {
		return p.testError(start, "NOT_FOUND", fmt.Sprintf("Jenkins 返回 404，请检查 URL 配置: %s", conn.Endpoint)), nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return p.testError(start, "AUTHENTICATION_ERROR", "Jenkins 认证失败，请检查用户名和 Token"), nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return p.testError(start, "HTTP_ERROR", fmt.Sprintf("Jenkins 返回 HTTP %d: %s", resp.StatusCode, string(body))), nil
	}

	var info struct {
		JenkinsVersion string `json:"jenkinsVersion"`
		NodeName       string `json:"nodeName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return p.testError(start, "PARSE_FAILED", err.Error()), nil
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: fmt.Sprintf("Jenkins %s, 节点: %s", info.JenkinsVersion, info.NodeName),
	}, nil
}

func (p *JenkinsProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}
	return &JenkinsClient{
		BaseURL:           conn.Endpoint,
		HTTPClient:        p.httpClient,
		CredentialService: p.credentialService,
		Connection:        conn,
	}, nil
}

func (p *JenkinsProvider) Trigger(ctx context.Context, conn *connection.Connection, job string, parameters map[string]string) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	jc, ok := client.(*JenkinsClient)
	if !ok {
		return nil, fmt.Errorf("无效的 Jenkins 客户端")
	}
	return jc.TriggerBuild(ctx, job, parameters)
}

func (p *JenkinsProvider) GetBuild(ctx context.Context, conn *connection.Connection, job string, buildNumber int) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	jc, ok := client.(*JenkinsClient)
	if !ok {
		return nil, fmt.Errorf("无效的 Jenkins 客户端")
	}
	return jc.GetBuild(ctx, job, buildNumber)
}

func (p *JenkinsProvider) GetBuildLog(ctx context.Context, conn *connection.Connection, job string, buildNumber int) (string, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return "", err
	}
	jc, ok := client.(*JenkinsClient)
	if !ok {
		return "", fmt.Errorf("无效的 Jenkins 客户端")
	}
	return jc.GetBuildLog(ctx, job, buildNumber)
}

func (p *JenkinsProvider) addAuth(ctx context.Context, conn *connection.Connection, req *http.Request) error {
	if conn.CredentialID == nil {
		return nil
	}
	data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
	if err != nil {
		return err
	}
	username, hasUser := data["username"]
	token, hasToken := data["token"]
	if hasUser && hasToken {
		req.SetBasicAuth(username, token)
		return nil
	}
	if hasUser {
		password, hasPass := data["password"]
		if hasPass {
			req.SetBasicAuth(username, password)
			return nil
		}
	}
	return nil
}

func (p *JenkinsProvider) testError(start time.Time, code, msg string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    code,
		ErrorMessage: msg,
		CheckedAt:    time.Now(),
	}
}

type JenkinsClient struct {
	BaseURL           string
	HTTPClient        *http.Client
	CredentialService *connection.CredentialService
	Connection        *connection.Connection
}

func (c *JenkinsClient) TriggerBuild(ctx context.Context, job string, parameters map[string]string) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/job/%s/build", c.BaseURL, job)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
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
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jenkins 触发构建失败: HTTP %d", resp.StatusCode)
	}
	return map[string]interface{}{"status": "triggered", "job": job}, nil
}

func (c *JenkinsClient) GetBuild(ctx context.Context, job string, buildNumber int) (map[string]interface{}, error) {
	reqURL := fmt.Sprintf("%s/job/%s/%d/api/json", c.BaseURL, job, buildNumber)
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
		return nil, fmt.Errorf("Jenkins 获取构建失败: HTTP %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *JenkinsClient) GetBuildLog(ctx context.Context, job string, buildNumber int) (string, error) {
	reqURL := fmt.Sprintf("%s/job/%s/%d/consoleText", c.BaseURL, job, buildNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	if err := c.addAuth(ctx, req); err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Jenkins 获取日志失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *JenkinsClient) addAuth(ctx context.Context, req *http.Request) error {
	if c.Connection.CredentialID == nil {
		return nil
	}
	data, err := c.CredentialService.GetDecryptedData(ctx, *c.Connection.CredentialID)
	if err != nil {
		return err
	}
	username, hasUser := data["username"]
	token, hasToken := data["token"]
	if hasUser && hasToken {
		req.SetBasicAuth(username, token)
		return nil
	}
	return nil
}
