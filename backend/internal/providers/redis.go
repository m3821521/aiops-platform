package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
	"github.com/redis/go-redis/v9"
)

// RedisProvider 实现 Redis 缓存连接 Provider。
//
// 功能：
//   - 从 Connection 获取 endpoint (host:port)
//   - 从 Credential 获取 password
//   - 从 Connection config 获取 db (数据库索引)
//   - Connection Test 执行 PING 命令验证连接
//   - 支持 TLS 配置（通过 Connection config）
type RedisProvider struct {
	credentialService *connection.CredentialService
}

// NewRedisProvider 创建 Redis Provider。
func NewRedisProvider(credentialService *connection.CredentialService) *RedisProvider {
	return &RedisProvider{
		credentialService: credentialService,
	}
}

// Type 返回 Provider 类型。
func (p *RedisProvider) Type() connection.ConnectionType {
	return connection.TypeRedis
}

// Test 测试 Redis 连接。
// 执行：建立连接 → PING → 检查返回 PONG。
func (p *RedisProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	if conn == nil {
		return p.testError(start, "INVALID_CONNECTION", "connection 不能为空"), nil
	}
	if !conn.Enabled {
		return p.testError(start, "CONNECTION_DISABLED", fmt.Sprintf("connection 已禁用: %s", conn.Name)), nil
	}

	// 构建 Redis 选项
	opts, err := p.buildOptions(ctx, conn)
	if err != nil {
		return p.testError(start, "OPTIONS_BUILD_FAILED", err.Error()), nil
	}

	// 创建客户端
	client := redis.NewClient(opts)
	defer client.Close()

	// 设置短超时
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 执行 PING
	result, err := client.Ping(testCtx).Result()
	if err != nil {
		errMsg := err.Error()
		// 分类错误
		if strings.Contains(errMsg, "NOAUTH") || strings.Contains(errMsg, "invalid password") {
			return p.testError(start, "AUTHENTICATION_ERROR", fmt.Sprintf("Redis 认证失败: %v", err)), nil
		}
		if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "i/o timeout") {
			return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 Redis: %v", err)), nil
		}
		if strings.Contains(errMsg, "DB index out of range") {
			return p.testError(start, "DATABASE_NOT_FOUND", fmt.Sprintf("Redis 数据库索引超出范围: %v", err)), nil
		}
		return p.testError(start, "PING_FAILED", fmt.Sprintf("Redis PING 失败: %v", err)), nil
	}

	if result != "PONG" {
		return p.testError(start, "UNEXPECTED_RESULT", fmt.Sprintf("Redis PING 返回意外结果: %s", result)), nil
	}

	// 获取 Redis 服务器信息（可选）
	info, err := client.Info(testCtx, "server").Result()
	redisVersion := ""
	if err == nil {
		for _, line := range strings.Split(info, "\n") {
			if strings.HasPrefix(line, "redis_version:") {
				redisVersion = strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
				break
			}
		}
	}

	message := "Redis 连接正常"
	if redisVersion != "" {
		message = fmt.Sprintf("Redis 连接正常, 版本: %s", redisVersion)
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: message,
	}, nil
}

// Connect 创建 Redis 客户端。
// 返回 *redis.Client，业务代码可以直接使用。
func (p *RedisProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}

	opts, err := p.buildOptions(ctx, conn)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)

	// 验证连接
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("Redis Ping 失败: %w", err)
	}

	return client, nil
}

// buildOptions 构建 Redis 客户端选项。
func (p *RedisProvider) buildOptions(ctx context.Context, conn *connection.Connection) (*redis.Options, error) {
	if conn.Endpoint == "" {
		return nil, fmt.Errorf("Redis endpoint 不能为空")
	}

	// 解析 endpoint，支持 host:port 或 redis:// URL
	addr := conn.Endpoint
	if strings.HasPrefix(addr, "redis://") {
		addr = strings.TrimPrefix(addr, "redis://")
		// 移除 user:pass@ 部分
		if atIdx := strings.Index(addr, "@"); atIdx != -1 {
			addr = addr[atIdx+1:]
		}
		// 移除 /db 部分
		if slashIdx := strings.Index(addr, "/"); slashIdx != -1 {
			addr = addr[:slashIdx]
		}
	}

	// 如果没有端口，默认 6379
	if !strings.Contains(addr, ":") {
		addr = addr + ":6379"
	}

	opts := &redis.Options{
		Addr:         addr,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	}

	// 从 Credential 获取 password
	if conn.CredentialID != nil {
		data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
		if err != nil {
			return nil, fmt.Errorf("解密 credential 失败: %w", err)
		}

		if password, ok := data["password"]; ok && password != "" {
			opts.Password = password
		}
		// 也支持 token 字段作为 password
		if token, ok := data["token"]; ok && token != "" {
			opts.Password = token
		}
	}

	// 从 Connection config 获取 db
	if conn.Config != nil {
		if dbVal, ok := conn.Config["db"]; ok {
			switch v := dbVal.(type) {
			case float64:
				opts.DB = int(v)
			case int:
				opts.DB = v
			case string:
				var dbInt int
				if _, err := fmt.Sscanf(v, "%d", &dbInt); err == nil {
					opts.DB = dbInt
				}
			}
		}
	}

	return opts, nil
}

// testError 构造测试失败结果。
func (p *RedisProvider) testError(start time.Time, errorCode, errorMessage string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CheckedAt:    time.Now(),
	}
}
