package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JenkinsClient 是 Jenkins REST API 客户端封装。
// 使用标准库 net/http，支持 Basic Auth 和 CSRF crumb。
type JenkinsClient struct {
	http     *http.Client
	baseURL  string
	username string
	token    string
}

// NewJenkinsClient 创建 Jenkins 客户端。
func NewJenkinsClient(baseURL, username, token string, timeoutSec int) *JenkinsClient {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &JenkinsClient{
		http:     &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		token:    token,
	}
}

// JenkinsJob 是 Jenkins Job 摘要。
type JenkinsJob struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Color     string `json:"color"` // blue=成功, red=失败, aborted, notbuilt 等
	LastBuild *struct {
		Number int    `json:"number"`
		Result string `json:"result"`
	} `json:"lastBuild,omitempty"`
}

// JenkinsBuild 是 Jenkins 构建信息。
type JenkinsBuild struct {
	Number    int    `json:"number"`
	Result    string `json:"result"` // SUCCESS, FAILURE, ABORTED, null(构建中)
	Timestamp int64  `json:"timestamp"`
	Duration  int64  `json:"duration"` // 毫秒
	Building  bool   `json:"building"`
	URL       string `json:"url"`
}

// ListJobs 列出所有 Job。
func (c *JenkinsClient) ListJobs(ctx context.Context) ([]JenkinsJob, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("Jenkins 未配置")
	}

	apiURL := c.baseURL + "/api/json?tree=jobs[name,url,color,lastBuild[number,result]]"
	var result struct {
		Jobs []JenkinsJob `json:"jobs"`
	}
	if err := c.doGet(ctx, apiURL, &result); err != nil {
		return nil, err
	}
	return result.Jobs, nil
}

// ListBuilds 列出指定 Job 的构建历史。
func (c *JenkinsClient) ListBuilds(ctx context.Context, jobName string) ([]JenkinsBuild, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("Jenkins 未配置")
	}

	apiURL := fmt.Sprintf("%s/job/%s/api/json?tree=builds[number,result,timestamp,duration,building,url]",
		c.baseURL, url.PathEscape(jobName))
	var result struct {
		Builds []JenkinsBuild `json:"builds"`
	}
	if err := c.doGet(ctx, apiURL, &result); err != nil {
		return nil, err
	}
	return result.Builds, nil
}

// Build 触发 Job 构建（写操作）。
func (c *JenkinsClient) Build(ctx context.Context, jobName string) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("Jenkins 未配置")
	}

	// 获取 CSRF crumb。
	crumb, err := c.getCrumb(ctx)
	if err != nil {
		return fmt.Errorf("获取 Jenkins crumb 失败: %w", err)
	}

	apiURL := fmt.Sprintf("%s/job/%s/build", c.baseURL, url.PathEscape(jobName))
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.token)
	if crumb != "" {
		req.Header.Set("Jenkins-Crumb", crumb)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Jenkins 构建返回 %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// GetBuildLog 获取构建日志。
func (c *JenkinsClient) GetBuildLog(ctx context.Context, jobName string, buildNumber int) (string, error) {
	if c == nil || c.baseURL == "" {
		return "", fmt.Errorf("Jenkins 未配置")
	}

	apiURL := fmt.Sprintf("%s/job/%s/%d/consoleText", c.baseURL, url.PathEscape(jobName), buildNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.username, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Jenkins 返回 %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getCrumb 获取 Jenkins CSRF crumb。
func (c *JenkinsClient) getCrumb(ctx context.Context) (string, error) {
	apiURL := c.baseURL + "/crumbIssuer/api/json"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.username, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Jenkins 未启用 CSRF，返回空。
		return "", nil
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("crumb 请求返回 %d", resp.StatusCode)
	}

	var result struct {
		Crumb string `json:"crumb"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Crumb, nil
}

// doGet 发送 GET 请求并解析 JSON。
func (c *JenkinsClient) doGet(ctx context.Context, apiURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Jenkins 返回 %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
