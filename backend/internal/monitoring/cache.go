package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Querier 是 Prometheus 查询能力的接口抽象。
// *Client 和 *CachedQuerier 都实现此接口，handler 层只依赖接口，便于切换/测试。
type Querier interface {
	Query(ctx context.Context, query string, ts time.Time) (*QueryResult, error)
	QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error)
	NodeMetrics(ctx context.Context) (*NodeMetrics, error)
	PodMetrics(ctx context.Context, namespace string) (*PodMetrics, error)
}

// 缓存 TTL。即时查询数据变化快，TTL 短；范围查询结果固定，TTL 长。
const (
	cacheTTLQuery  = 15 * time.Second
	cacheTTLR      = 60 * time.Second
	cacheTTLPreset = 30 * time.Second
)

// CachedQuerier 在 Querier 外层包一层 Redis 缓存。
// Redis 不可用时自动降级为直接查询，不影响功能。
type CachedQuerier struct {
	inner Querier
	rdb   *redis.Client
}

// NewCachedQuerier 创建带 Redis 缓存的查询器。rdb 为 nil 时直接返回 inner（无缓存）。
func NewCachedQuerier(inner Querier, rdb *redis.Client) Querier {
	if rdb == nil {
		return inner
	}
	return &CachedQuerier{inner: inner, rdb: rdb}
}

func (c *CachedQuerier) Query(ctx context.Context, query string, ts time.Time) (*QueryResult, error) {
	key := cacheKey("q", query, fmt.Sprintf("%d", ts.Unix()))
	return getOrSet(ctx, c.rdb, key, cacheTTLQuery, func() (*QueryResult, error) {
		return c.inner.Query(ctx, query, ts)
	})
}

func (c *CachedQuerier) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error) {
	key := cacheKey("r", query, fmt.Sprintf("%d-%d-%d", start.Unix(), end.Unix(), int(step.Seconds())))
	return getOrSet(ctx, c.rdb, key, cacheTTLR, func() (*QueryResult, error) {
		return c.inner.QueryRange(ctx, query, start, end, step)
	})
}

func (c *CachedQuerier) NodeMetrics(ctx context.Context) (*NodeMetrics, error) {
	key := "prom:nodes"
	return getOrSet(ctx, c.rdb, key, cacheTTLPreset, func() (*NodeMetrics, error) {
		return c.inner.NodeMetrics(ctx)
	})
}

func (c *CachedQuerier) PodMetrics(ctx context.Context, namespace string) (*PodMetrics, error) {
	key := "prom:pods:" + namespace
	return getOrSet(ctx, c.rdb, key, cacheTTLPreset, func() (*PodMetrics, error) {
		return c.inner.PodMetrics(ctx, namespace)
	})
}

// cacheKey 生成缓存 key：前缀 + sha256(参数拼接)，避免 key 过长或含特殊字符。
func cacheKey(prefix, query, extra string) string {
	h := sha256.Sum256([]byte(query + "|" + extra))
	return "prom:" + prefix + ":" + hex.EncodeToString(h[:])
}

// getOrSet 是泛型缓存辅助函数：先查 Redis，命中则反序列化返回；未命中则调用 fn 并写入缓存。
// Redis 操作失败时降级为直接调用 fn，不报错。
func getOrSet[T any](ctx context.Context, rdb *redis.Client, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	var zero T

	// 尝试读缓存。
	if data, err := rdb.Get(ctx, key).Bytes(); err == nil {
		var v T
		if json.Unmarshal(data, &v) == nil {
			return v, nil
		}
	}

	// 未命中或反序列化失败，调用实际查询。
	v, err := fn()
	if err != nil {
		return zero, err
	}

	// 写入缓存，失败不影响返回。
	if data, err := json.Marshal(v); err == nil {
		_ = rdb.Set(ctx, key, data, ttl).Err()
	}
	return v, nil
}
