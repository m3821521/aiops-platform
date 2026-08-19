package redisutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Lock 是基于 Redis 的分布式锁。
type Lock struct {
	client *redis.Client
	key    string
	value  string
	ttl    time.Duration
}

// NewLock 创建分布式锁。
func NewLock(client *redis.Client, key string, ttl time.Duration) *Lock {
	return &Lock{
		client: client,
		key:    "lock:" + key,
		value:  uuid.NewString(),
		ttl:    ttl,
	}
}

// Acquire 尝试获取锁。
// 如果锁已被占用，返回 false。
func (l *Lock) Acquire(ctx context.Context) (bool, error) {
	if l.client == nil {
		return false, errors.New("Redis 客户端未初始化")
	}

	ok, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("获取锁失败: %w", err)
	}
	return ok, nil
}

// Release 释放锁。
// 只有锁的持有者才能释放（通过 value 验证）。
func (l *Lock) Release(ctx context.Context) error {
	if l.client == nil {
		return errors.New("Redis 客户端未初始化")
	}

	// 使用 Lua 脚本保证原子性：检查 value 匹配后删除。
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	_, err := l.client.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("释放锁失败: %w", err)
	}
	return nil
}

// AcquireWithRetry 带重试的获取锁。
// retryCount 重试次数，retryInterval 重试间隔。
func (l *Lock) AcquireWithRetry(ctx context.Context, retryCount int, retryInterval time.Duration) (bool, error) {
	for i := 0; i < retryCount; i++ {
		ok, err := l.Acquire(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(retryInterval):
		}
	}
	return false, nil
}
