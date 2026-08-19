package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// maxQueryLen 限制 PromQL 长度，防止超长查询拖垮 Prometheus。
const maxQueryLen = 4096

// QueryResult 对齐 Prometheus HTTP API 的 data 段格式，前端可直接解析。
type QueryResult struct {
	ResultType string `json:"resultType"` // vector / matrix / scalar / string
	Result     any    `json:"result"`
}

// Client 封装 Prometheus HTTP API。
// 不做启动时连通性检查：Prometheus 是可选依赖，连不上时查询接口返回错误，不影响进程启动。
type Client struct {
	api     v1.API
	address string
	timeout time.Duration
}

// NewClient 创建 Prometheus 客户端。timeout 是单次查询的最长等待时间。
func NewClient(address string, timeout time.Duration) (*Client, error) {
	if address == "" {
		return nil, fmt.Errorf("prometheus address 不能为空")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	c, err := api.NewClient(api.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("创建 Prometheus 客户端失败: %w", err)
	}
	return &Client{
		api:     v1.NewAPI(c),
		address: address,
		timeout: timeout,
	}, nil
}

// Address 返回配置的 Prometheus 地址，用于健康检查或调试展示。
func (c *Client) Address() string {
	return c.address
}

// Query 执行即时查询（instant query）。
// ts 为查询时间点，传零值时 Prometheus 使用当前时间。
func (c *Client) Query(ctx context.Context, query string, ts time.Time) (*QueryResult, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result, _, err := c.api.Query(ctx, query, ts)
	if err != nil {
		return nil, fmt.Errorf("Prometheus 查询失败: %w", err)
	}
	return &QueryResult{
		ResultType: result.Type().String(),
		Result:     result,
	}, nil
}

// QueryRange 执行范围查询（range query），返回 matrix 类型。
// step 是数据点间隔，如 15s、1m。
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("start 和 end 不能为空")
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end 必须晚于 start")
	}
	if step <= 0 {
		return nil, fmt.Errorf("step 必须大于 0")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result, _, err := c.api.QueryRange(ctx, query, v1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("Prometheus 范围查询失败: %w", err)
	}
	return &QueryResult{
		ResultType: result.Type().String(),
		Result:     result,
	}, nil
}

// validateQuery 统一校验 PromQL 输入。
func validateQuery(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("query 不能为空")
	}
	if len(query) > maxQueryLen {
		return fmt.Errorf("query 过长（%d > %d）", len(query), maxQueryLen)
	}
	return nil
}
