package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 自动迁移表结构。
	if err := db.AutoMigrate(&auth.User{}, &auth.Role{}, &auth.Permission{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoginSuccess(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)

	// 创建测试用户。
	hash, _ := auth.HashPassword("password123")
	user := &auth.User{
		Username:     "testuser",
		PasswordHash: hash,
		Status:       "active",
		Roles:        []auth.Role{{Name: "viewer"}},
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}

	svc := auth.NewService(repo, "test-secret", 1*time.Hour)
	result, err := svc.Login(context.Background(), auth.LoginRequest{
		Username: "testuser",
		Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.TokenType != "Bearer" {
		t.Fatalf("expected Bearer, got %s", result.TokenType)
	}
	if result.User == nil || result.User.Username != "testuser" {
		t.Fatal("expected user in response")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)

	hash, _ := auth.HashPassword("correct")
	db.Create(&auth.User{Username: "user", PasswordHash: hash, Status: "active"})

	svc := auth.NewService(repo, "secret", time.Hour)
	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Username: "user",
		Password: "wrong",
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestLoginUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)
	svc := auth.NewService(repo, "secret", time.Hour)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Username: "nonexistent",
		Password: "password",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestLoginDisabledUser(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)

	hash, _ := auth.HashPassword("password")
	db.Create(&auth.User{Username: "disabled", PasswordHash: hash, Status: "disabled"})

	svc := auth.NewService(repo, "secret", time.Hour)
	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Username: "disabled",
		Password: "password",
	})
	if err == nil {
		t.Fatal("expected error for disabled user")
	}
}

func TestLoginEmptyCredentials(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)
	svc := auth.NewService(repo, "secret", time.Hour)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Username: "",
		Password: "",
	})
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
}

func TestValidateToken(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)

	hash, _ := auth.HashPassword("password")
	user := &auth.User{Username: "validuser", PasswordHash: hash, Status: "active"}
	db.Create(user)

	svc := auth.NewService(repo, "test-secret", time.Hour)

	// 登录获取 token。
	loginResult, err := svc.Login(context.Background(), auth.LoginRequest{
		Username: "validuser",
		Password: "password",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证 token。
	validatedUser, err := svc.ValidateToken(context.Background(), loginResult.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if validatedUser.Username != "validuser" {
		t.Fatalf("expected username validuser, got %s", validatedUser.Username)
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	db := setupTestDB(t)
	repo := auth.NewRepository(db)
	svc := auth.NewService(repo, "secret", time.Hour)

	_, err := svc.ValidateToken(context.Background(), "invalid.token.here")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}
