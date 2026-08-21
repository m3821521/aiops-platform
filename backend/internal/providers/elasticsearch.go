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
	"github.com/aiops/aiops-platform/internal/ssrf"
)

// ElasticsearchProvider 实现 connection.LogProvider 接口。
type ElasticsearchProvider struct {
	credentialService *connection.CredentialService
	httpClient        *http.Client
}

func NewElasticsearchProvider(credentialService *connection.CredentialService) *ElasticsearchProvider {
	return &ElasticsearchProvider{
		credentialService: credentialService,
		// P0-03: 使用 SafeTransport 防止 SSRF
		httpClient: ssrf.NewSafeTransport(ssrf.DefaultConfig()).HTTPClient(),
	}
}

func (p *ElasticsearchProvider) Type() connection.ConnectionType {
	return connection.TypeElasticsearch
}

func (p *ElasticsearchProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	reqURL := fmt.Sprintf("%s/_cluster/health", conn.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return p.testError(start, "REQUEST_BUILD_FAILED", err.Error()), nil
	}

	if err := p.addAuth(ctx, conn, req); err != nil {
		return p.testError(start, "AUTH_FAILED", err.Error()), nil
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 Elasticsearch: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return p.testError(start, "HTTP_ERROR", fmt.Sprintf("Elasticsearch 返回 HTTP %d: %s", resp.StatusCode, string(body))), nil
	}

	var health struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return p.testError(start, "PARSE_FAILED", err.Error()), nil
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: fmt.Sprintf("Elasticsearch 状态: %s", health.Status),
	}, nil
}

func (p *ElasticsearchProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}
	return &ElasticsearchClient{
		BaseURL:           conn.Endpoint,
		HTTPClient:        p.httpClient,
		CredentialService: p.credentialService,
		Connection:        conn,
	}, nil
}

func (p *ElasticsearchProvider) Search(ctx context.Context, conn *connection.Connection, query string, startTime, endTime time.Time, limit int) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	ec, ok := client.(*ElasticsearchClient)
	if !ok {
		return nil, fmt.Errorf("无效的 Elasticsearch 客户端")
	}
	return ec.Search(ctx, query, startTime, endTime, limit)
}

func (p *ElasticsearchProvider) addAuth(ctx context.Context, conn *connection.Connection, req *http.Request) error {
	if conn.CredentialID == nil {
		return nil
	}
	data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
	if err != nil {
		return err
	}
	if apiKey, ok := data["api_key"]; ok && apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+apiKey)
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

func (p *ElasticsearchProvider) testError(start time.Time, code, msg string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    code,
		ErrorMessage: msg,
		CheckedAt:    time.Now(),
	}
}

type ElasticsearchClient struct {
	BaseURL           string
	HTTPClient        *http.Client
	CredentialService *connection.CredentialService
	Connection        *connection.Connection
}

func (c *ElasticsearchClient) Search(ctx context.Context, query string, startTime, endTime time.Time, limit int) (map[string]interface{}, error) {
	index := "filebeat-*"
	if c.Connection.Config != nil {
		if idx, ok := c.Connection.Config["index"]; ok && idx != "" {
			index = fmt.Sprintf("%v", idx)
		}
	}

	body := map[string]interface{}{
		"size": limit,
		"sort": []map[string]interface{}{
			{"@timestamp": map[string]string{"order": "desc"}},
		},
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"query_string": map[string]string{"query": query}},
				},
				"filter": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"@timestamp": map[string]string{
								"gte": startTime.Format(time.RFC3339),
								"lte": endTime.Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(body)
	reqURL := fmt.Sprintf("%s/%s/_search", c.BaseURL, index)
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
		return nil, fmt.Errorf("Elasticsearch 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Elasticsearch 返回 HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ElasticsearchClient) addAuth(ctx context.Context, req *http.Request) error {
	if c.Connection.CredentialID == nil {
		return nil
	}
	data, err := c.CredentialService.GetDecryptedData(ctx, *c.Connection.CredentialID)
	if err != nil {
		return err
	}
	if apiKey, ok := data["api_key"]; ok && apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+apiKey)
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
