package logging

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

// Client 是 Elasticsearch 客户端封装。
// 使用标准库 net/http，不引入额外依赖，保持简单可控。
type Client struct {
	http     *http.Client
	address  string
	index    string
	username string
	password string
}

// NewClient 创建 Elasticsearch 客户端。
// 不做连通性检查（ES 是可选依赖），调用时才报错。
func NewClient(address, index, username, password string, timeoutSec int) *Client {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &Client{
		http:     &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		address:  strings.TrimRight(address, "/"),
		index:    index,
		username: username,
		password: password,
	}
}

// SearchQuery 是日志查询参数。
type SearchQuery struct {
	Keyword   string    // 全文搜索关键词
	Namespace string    // 命名空间过滤
	Pod       string    // Pod 过滤
	Container string    // 容器过滤
	Level     string    // 日志级别过滤（error/warn/info/debug）
	TraceID   string    // 追踪 ID
	RequestID string    // 请求 ID
	Start     time.Time // 开始时间
	End       time.Time // 结束时间
	From      int       // 分页偏移
	Size      int       // 每页大小
}

// LogHit 是单条日志命中。
type LogHit struct {
	Index     string                 `json:"_index"`
	ID        string                 `json:"_id"`
	Score     float64                `json:"_score"`
	Source    map[string]interface{} `json:"_source"`
	Timestamp time.Time              `json:"@timestamp,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Level     string                 `json:"level,omitempty"`
	Namespace string                 `json:"namespace,omitempty"`
	Pod       string                 `json:"pod,omitempty"`
	Container string                 `json:"container,omitempty"`
}

// SearchResult 是查询结果。
type SearchResult struct {
	Total int64    `json:"total"`
	Hits  []LogHit `json:"hits"`
	Took  int64    `json:"took"`
}

// Search 执行日志查询。
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if c == nil || c.address == "" {
		return nil, fmt.Errorf("elasticsearch 未配置")
	}

	body := buildQueryDSL(q)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化查询失败: %w", err)
	}

	url := fmt.Sprintf("%s/%s/_search", c.address, c.index)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Elasticsearch 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Elasticsearch 返回 %d: %s", resp.StatusCode, string(respBytes))
	}

	return parseSearchResponse(respBytes)
}

// esQuery 是 Elasticsearch 查询 DSL 的内部结构。
type esQuery struct {
	Query esBoolQuery `json:"query"`
	Sort  []esSort    `json:"sort"`
	From  int         `json:"from"`
	Size  int         `json:"size"`
}

type esBoolQuery struct {
	Bool esBool `json:"bool"`
}

type esBool struct {
	Must   []map[string]interface{} `json:"must,omitempty"`
	Filter []map[string]interface{} `json:"filter,omitempty"`
}

type esSort map[string]string

// buildQueryDSL 构建 Elasticsearch 查询 DSL。
func buildQueryDSL(q SearchQuery) esQuery {
	query := esQuery{
		Sort: []esSort{{"@timestamp": "desc"}},
		From: q.From,
		Size: q.Size,
	}
	if query.Size <= 0 {
		query.Size = 100
	}
	if query.Size > 10000 {
		query.Size = 10000
	}

	var must []map[string]interface{}
	var filter []map[string]interface{}

	// 全文搜索。
	if q.Keyword != "" {
		must = append(must, map[string]interface{}{
			"match": map[string]interface{}{
				"message": q.Keyword,
			},
		})
	}

	// 命名空间过滤（兼容 Filebeat 和 Fluentd 字段名）。
	if q.Namespace != "" {
		filter = append(filter, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{"term": map[string]string{"kubernetes.namespace.keyword": q.Namespace}},
					{"term": map[string]string{"kubernetes.namespace": q.Namespace}},
					{"term": map[string]string{"namespace": q.Namespace}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// Pod 过滤。
	if q.Pod != "" {
		filter = append(filter, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{"term": map[string]string{"kubernetes.pod.name.keyword": q.Pod}},
					{"term": map[string]string{"kubernetes.pod.name": q.Pod}},
					{"term": map[string]string{"pod": q.Pod}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// 容器过滤。
	if q.Container != "" {
		filter = append(filter, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{"term": map[string]string{"kubernetes.container.name.keyword": q.Container}},
					{"term": map[string]string{"container.name": q.Container}},
					{"term": map[string]string{"container": q.Container}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// 日志级别过滤。
	if q.Level != "" {
		filter = append(filter, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{"term": map[string]string{"log.level.keyword": q.Level}},
					{"term": map[string]string{"level": q.Level}},
					{"term": map[string]string{"level.keyword": q.Level}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// Trace ID 过滤。
	if q.TraceID != "" {
		filter = append(filter, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{"term": map[string]string{"trace.id": q.TraceID}},
					{"term": map[string]string{"trace_id": q.TraceID}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// Request ID 过滤。
	if q.RequestID != "" {
		filter = append(filter, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{"term": map[string]string{"request.id": q.RequestID}},
					{"term": map[string]string{"request_id": q.RequestID}},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// 时间范围过滤。
	if !q.Start.IsZero() || !q.End.IsZero() {
		rangeQ := map[string]interface{}{}
		if !q.Start.IsZero() {
			rangeQ["gte"] = q.Start.Format(time.RFC3339)
		}
		if !q.End.IsZero() {
			rangeQ["lte"] = q.End.Format(time.RFC3339)
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": rangeQ,
			},
		})
	}

	query.Query.Bool.Must = must
	query.Query.Bool.Filter = filter
	return query
}

// esResponse 是 Elasticsearch 搜索响应的内部结构。
type esResponse struct {
	Took int64 `json:"took"`
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []struct {
			Index  string                 `json:"_index"`
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

// parseSearchResponse 解析 Elasticsearch 响应。
func parseSearchResponse(data []byte) (*SearchResult, error) {
	var resp esResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	result := &SearchResult{
		Total: resp.Hits.Total.Value,
		Took:  resp.Took,
		Hits:  make([]LogHit, 0, len(resp.Hits.Hits)),
	}

	for _, h := range resp.Hits.Hits {
		hit := LogHit{
			Index:  h.Index,
			ID:     h.ID,
			Score:  h.Score,
			Source: h.Source,
		}
		// 从 _source 中提取常用字段。
		if v, ok := h.Source["@timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				hit.Timestamp = t
			}
		}
		if v, ok := h.Source["message"].(string); ok {
			hit.Message = v
		}
		// 兼容多种 level 字段。
		if v, ok := h.Source["log"].(map[string]interface{}); ok {
			if l, ok := v["level"].(string); ok {
				hit.Level = l
			}
		}
		if hit.Level == "" {
			if v, ok := h.Source["level"].(string); ok {
				hit.Level = v
			}
		}
		// 兼容多种 namespace 字段。
		if v, ok := h.Source["kubernetes"].(map[string]interface{}); ok {
			if ns, ok := v["namespace"].(string); ok {
				hit.Namespace = ns
			}
			if pod, ok := v["pod"].(map[string]interface{}); ok {
				if name, ok := pod["name"].(string); ok {
					hit.Pod = name
				}
			}
			if container, ok := v["container"].(map[string]interface{}); ok {
				if name, ok := container["name"].(string); ok {
					hit.Container = name
				}
			}
		}
		if hit.Namespace == "" {
			if v, ok := h.Source["namespace"].(string); ok {
				hit.Namespace = v
			}
		}
		if hit.Pod == "" {
			if v, ok := h.Source["pod"].(string); ok {
				hit.Pod = v
			}
		}
		if hit.Container == "" {
			if v, ok := h.Source["container"].(string); ok {
				hit.Container = v
			}
		}
		result.Hits = append(result.Hits, hit)
	}

	return result, nil
}
