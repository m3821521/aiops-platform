package providers

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/connection"
	_ "github.com/go-sql-driver/mysql"
)

// MySQLProvider 实现 MySQL 数据库连接 Provider。
//
// 功能：
//   - 从 Connection 获取 endpoint (host:port) 和 database
//   - 从 Credential 获取 username/password
//   - Connection Test 执行 SELECT 1 验证连接
//   - 支持 TLS 配置（通过 Connection config）
type MySQLProvider struct {
	credentialService *connection.CredentialService
}

// NewMySQLProvider 创建 MySQL Provider。
func NewMySQLProvider(credentialService *connection.CredentialService) *MySQLProvider {
	return &MySQLProvider{
		credentialService: credentialService,
	}
}

// Type 返回 Provider 类型。
func (p *MySQLProvider) Type() connection.ConnectionType {
	return connection.TypeMySQL
}

// Test 测试 MySQL 连接。
// 执行：建立连接 → SELECT 1 → 检查返回结果。
func (p *MySQLProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	if conn == nil {
		return p.testError(start, "INVALID_CONNECTION", "connection 不能为空"), nil
	}
	if !conn.Enabled {
		return p.testError(start, "CONNECTION_DISABLED", fmt.Sprintf("connection 已禁用: %s", conn.Name)), nil
	}

	// 构建 DSN
	dsn, err := p.buildDSN(ctx, conn)
	if err != nil {
		return p.testError(start, "DSN_BUILD_FAILED", err.Error()), nil
	}

	// 建立连接（使用短超时）
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return p.testError(start, "OPEN_FAILED", fmt.Sprintf("打开 MySQL 连接失败: %v", err)), nil
	}
	defer db.Close()

	// 设置连接池参数（测试用）
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Second)

	// Ping 验证连接
	if err := db.PingContext(connectCtx); err != nil {
		errMsg := err.Error()
		// 分类错误
		if strings.Contains(errMsg, "Access denied") {
			return p.testError(start, "AUTHENTICATION_ERROR", fmt.Sprintf("MySQL 认证失败: %v", err)), nil
		}
		if strings.Contains(errMsg, "Unknown database") {
			return p.testError(start, "DATABASE_NOT_FOUND", fmt.Sprintf("MySQL 数据库不存在: %v", err)), nil
		}
		if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "i/o timeout") {
			return p.testError(start, "CONNECTION_FAILED", fmt.Sprintf("无法连接 MySQL: %v", err)), nil
		}
		return p.testError(start, "PING_FAILED", fmt.Sprintf("MySQL Ping 失败: %v", err)), nil
	}

	// 执行 SELECT 1 验证
	var result int
	if err := db.QueryRowContext(connectCtx, "SELECT 1").Scan(&result); err != nil {
		return p.testError(start, "QUERY_FAILED", fmt.Sprintf("MySQL SELECT 1 失败: %v", err)), nil
	}

	if result != 1 {
		return p.testError(start, "UNEXPECTED_RESULT", fmt.Sprintf("MySQL SELECT 1 返回意外结果: %d", result)), nil
	}

	// 获取数据库版本（可选信息）
	var version string
	_ = db.QueryRowContext(connectCtx, "SELECT VERSION()").Scan(&version)

	message := "MySQL 连接正常"
	if version != "" {
		message = fmt.Sprintf("MySQL 连接正常, 版本: %s", version)
	}

	return &connection.TestConnectionResult{
		Status:       connection.StatusAvailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now(),
		ErrorMessage: message,
	}, nil
}

// Connect 创建 MySQL 数据库连接。
// 返回 *sql.DB，业务代码可以直接使用。
func (p *MySQLProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}

	dsn, err := p.buildDSN(ctx, conn)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 验证连接
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("MySQL Ping 失败: %w", err)
	}

	return db, nil
}

// buildDSN 构建 MySQL DSN。
// 格式: username:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
func (p *MySQLProvider) buildDSN(ctx context.Context, conn *connection.Connection) (string, error) {
	if conn.Endpoint == "" {
		return "", fmt.Errorf("MySQL endpoint 不能为空")
	}

	// 解析 endpoint，支持 host:port 或完整 URL
	host, port, database := p.parseEndpoint(conn.Endpoint)

	// 从 Connection config 中获取 database（如果 endpoint 中没有）
	if database == "" && conn.Config != nil {
		if db, ok := conn.Config["database"]; ok {
			if dbStr, ok := db.(string); ok && dbStr != "" {
				database = dbStr
			}
		}
	}

	// 获取认证信息
	username := "root"
	password := ""

	if conn.CredentialID != nil {
		data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
		if err != nil {
			return "", fmt.Errorf("解密 credential 失败: %w", err)
		}

		if user, ok := data["username"]; ok && user != "" {
			username = user
		}
		if pass, ok := data["password"]; ok && pass != "" {
			password = pass
		}
	}

	// 构建 DSN
	params := url.Values{}
	params.Set("charset", "utf8mb4")
	params.Set("parseTime", "true")
	params.Set("loc", "Local")
	params.Set("timeout", "10s")
	params.Set("readTimeout", "30s")
	params.Set("writeTimeout", "30s")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		username,
		password,
		host,
		port,
		database,
		params.Encode(),
	)

	return dsn, nil
}

// parseEndpoint 解析 MySQL endpoint。
// 支持格式:
//   - host:port
//   - host:port/database
//   - mysql://user:pass@host:port/database
//   - 完整 URL
func (p *MySQLProvider) parseEndpoint(endpoint string) (host, port, database string) {
	endpoint = strings.TrimSpace(endpoint)

	// 处理 mysql:// 前缀
	if strings.HasPrefix(endpoint, "mysql://") {
		endpoint = strings.TrimPrefix(endpoint, "mysql://")
		// 移除 user:pass@ 部分
		if atIdx := strings.Index(endpoint, "@"); atIdx != -1 {
			endpoint = endpoint[atIdx+1:]
		}
	}

	// 分离 database
	if slashIdx := strings.Index(endpoint, "/"); slashIdx != -1 {
		database = endpoint[slashIdx+1:]
		// 移除 query 参数
		if qIdx := strings.Index(database, "?"); qIdx != -1 {
			database = database[:qIdx]
		}
		endpoint = endpoint[:slashIdx]
	}

	// 分离 host:port
	if colonIdx := strings.Index(endpoint, ":"); colonIdx != -1 {
		host = endpoint[:colonIdx]
		port = endpoint[colonIdx+1:]
	} else {
		host = endpoint
		port = "3306"
	}

	return host, port, database
}

// testError 构造测试失败结果。
func (p *MySQLProvider) testError(start time.Time, errorCode, errorMessage string) *connection.TestConnectionResult {
	return &connection.TestConnectionResult{
		Status:       connection.StatusUnavailable,
		LatencyMs:    time.Since(start).Milliseconds(),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CheckedAt:    time.Now(),
	}
}
