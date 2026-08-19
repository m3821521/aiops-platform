package redisutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/redisutil"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupMiniRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return client, mr
}

func TestDistributedLockAcquireAndRelease(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	lock := redisutil.NewLock(client, "test-lock", 10*time.Second)

	// 获取锁。
	ok, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected to acquire lock")
	}

	// 再次获取应该失败。
	lock2 := redisutil.NewLock(client, "test-lock", 10*time.Second)
	ok, err = lock2.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected to NOT acquire lock (held by another)")
	}

	// 释放锁。
	if err := lock.Release(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 释放后应该可以获取。
	ok, err = lock2.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected to acquire lock after release")
	}
}

func TestDistributedLockReleaseByNonHolder(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	lock1 := redisutil.NewLock(client, "test-lock", 10*time.Second)
	lock2 := redisutil.NewLock(client, "test-lock", 10*time.Second)

	lock1.Acquire(context.Background())

	// 非持有者释放锁应该不影响（value 不匹配）。
	if err := lock2.Release(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 锁应该还在。
	ok, _ := lock2.Acquire(context.Background())
	if ok {
		t.Fatal("lock should still be held")
	}
}

func TestDistributedLockTTL(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	lock := redisutil.NewLock(client, "ttl-lock", 1*time.Second)
	lock.Acquire(context.Background())

	// 快进时间。
	mr.FastForward(2 * time.Second)

	// TTL 过期后应该可以获取。
	lock2 := redisutil.NewLock(client, "ttl-lock", 10*time.Second)
	ok, err := lock2.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected to acquire lock after TTL expiry")
	}
}

func TestDistributedLockNilClient(t *testing.T) {
	var client *redis.Client
	lock := redisutil.NewLock(client, "test", time.Second)
	_, err := lock.Acquire(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	rl := redisutil.NewRateLimiter(client, 3, time.Minute)

	for i := 0; i < 3; i++ {
		allowed, _, _, err := rl.Allow(context.Background(), "user-1")
		if err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 第 4 次应该被拒绝。
	allowed, remaining, _, err := rl.Allow(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("4th request should be rejected")
	}
	if remaining != 0 {
		t.Fatalf("expected remaining 0, got %d", remaining)
	}
}

func TestRateLimiterDifferentIdentifiers(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	rl := redisutil.NewRateLimiter(client, 2, time.Minute)

	// user-1 用满配额。
	rl.Allow(context.Background(), "user-1")
	rl.Allow(context.Background(), "user-1")

	// user-2 应该还能请求。
	allowed, _, _, err := rl.Allow(context.Background(), "user-2")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("user-2 should be allowed")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	rl := redisutil.NewRateLimiter(client, 2, time.Minute)

	rl.Allow(context.Background(), "user-1")
	rl.Allow(context.Background(), "user-1")

	// 快进到下一个窗口。
	mr.FastForward(2 * time.Minute)

	// 应该又能请求了。
	allowed, _, _, err := rl.Allow(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("should be allowed in new window")
	}
}

func TestRateLimiterNilClient(t *testing.T) {
	var client *redis.Client
	rl := redisutil.NewRateLimiter(client, 10, time.Minute)

	// nil client 时应该放行。
	allowed, _, _, err := rl.Allow(context.Background(), "user")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("should allow when client is nil")
	}
}

func TestRateLimiterDefaults(t *testing.T) {
	client, mr := setupMiniRedis(t)
	defer mr.Close()

	// 不传有效参数时使用默认值。
	rl := redisutil.NewRateLimiter(client, 0, 0)

	allowed, _, _, err := rl.Allow(context.Background(), "user")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("should allow with default limits")
	}
}
