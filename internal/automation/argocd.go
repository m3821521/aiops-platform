package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ArgoCDClient 是 ArgoCD API 客户端封装。
type ArgoCDClient struct {
	http    *http.Client
	baseURL string
	token   string
}

// NewArgoCDClient 创建 ArgoCD 客户端。
func NewArgoCDClient(baseURL, token string, timeoutSec int) *ArgoCDClient {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &ArgoCDClient{
		http:    &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
	}
}

// ArgoApplication 是 ArgoCD Application 摘要。
type ArgoApplication struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Project   string `json:"project"`
	Status    struct {
		Sync struct {
			Status string `json:"status"` // Synced, OutOfSync
		} `json:"sync"`
		Health struct {
			Status string `json:"status"` // Healthy, Degraded, Progressing, Missing
		} `json:"health"`
	} `json:"status"`
	Spec struct {
		Source struct {
			RepoURL string `json:"repoURL"`
			Path    string `json:"path"`
			Target  string `json:"targetRevision"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
	} `json:"spec"`
}

// ListApplications 列出所有 Application。
func (c *ArgoCDClient) ListApplications(ctx context.Context) ([]ArgoApplication, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("ArgoCD 未配置")
	}

	apiURL := c.baseURL + "/api/v1/applications"
	var result struct {
		Items []ArgoApplication `json:"items"`
	}
	if err := c.doRequest(ctx, "GET", apiURL, nil, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

// GetApplication 获取指定 Application 详情。
func (c *ArgoCDClient) GetApplication(ctx context.Context, name string) (*ArgoApplication, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("ArgoCD 未配置")
	}

	apiURL := fmt.Sprintf("%s/api/v1/applications/%s", c.baseURL, name)
	var app ArgoApplication
	if err := c.doRequest(ctx, "GET", apiURL, nil, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// Sync 触发 Application Sync（写操作）。
func (c *ArgoCDClient) Sync(ctx context.Context, name string) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("ArgoCD 未配置")
	}

	apiURL := fmt.Sprintf("%s/api/v1/applications/%s/sync", c.baseURL, name)
	// Sync 请求体可以为空，使用默认策略。
	body := map[string]interface{}{}
	return c.doRequest(ctx, "POST", apiURL, body, nil)
}

// Refresh 刷新 Application（写操作，轻量级）。
func (c *ArgoCDClient) Refresh(ctx context.Context, name string, hard bool) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("ArgoCD 未配置")
	}

	apiURL := fmt.Sprintf("%s/api/v1/applications/%s/refresh", c.baseURL, name)
	if hard {
		apiURL += "?hard=true"
	}
	return c.doRequest(ctx, "POST", apiURL, nil, nil)
}

// doRequest 发送 HTTP 请求并解析 JSON 响应。
func (c *ArgoCDClient) doRequest(ctx context.Context, method, url string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化请求失败: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求 ArgoCD 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ArgoCD 返回 %d: %s", resp.StatusCode, string(respBody))
	}

	if out != nil && resp.StatusCode != 204 {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
