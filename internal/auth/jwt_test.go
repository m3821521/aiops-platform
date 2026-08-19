package auth_test

import (
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/auth"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := auth.HashPassword("mysecret")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "mysecret" {
		t.Fatal("password should be hashed")
	}
	if !auth.CheckPassword("mysecret", hash) {
		t.Fatal("correct password should pass")
	}
	if auth.CheckPassword("wrongpassword", hash) {
		t.Fatal("wrong password should fail")
	}
}

func TestGenerateAndParseToken(t *testing.T) {
	user := &auth.User{
		ID:       1,
		Username: "admin",
		Roles: []auth.Role{
			{Name: "admin"},
		},
	}

	cfg := auth.JWTConfig{
		Secret:     "test-secret",
		Expiration: 1 * time.Hour,
	}

	token, err := auth.GenerateToken(user, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ParseToken(token, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 1 {
		t.Fatalf("expected user_id 1, got %d", claims.UserID)
	}
	if claims.Username != "admin" {
		t.Fatalf("expected username admin, got %s", claims.Username)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Fatalf("expected roles [admin], got %v", claims.Roles)
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	user := &auth.User{ID: 1, Username: "admin"}
	cfg := auth.JWTConfig{Secret: "secret1", Expiration: time.Hour}
	token, _ := auth.GenerateToken(user, cfg)

	_, err := auth.ParseToken(token, "secret2")
	if err == nil {
		t.Fatal("expected error with wrong secret")
	}
}

func TestParseTokenEmptySecret(t *testing.T) {
	_, err := auth.ParseToken("anytoken", "")
	if err == nil {
		t.Fatal("expected error with empty secret")
	}
}

func TestGenerateTokenEmptySecret(t *testing.T) {
	user := &auth.User{ID: 1}
	_, err := auth.GenerateToken(user, auth.JWTConfig{Secret: ""})
	if err == nil {
		t.Fatal("expected error with empty secret")
	}
}

func TestUserHasPermission(t *testing.T) {
	// admin 角色拥有所有权限。
	admin := &auth.User{
		Roles: []auth.Role{{Name: "admin"}},
	}
	if !admin.HasPermission("alerts", "read") {
		t.Fatal("admin should have all permissions")
	}

	// viewer 角色只有只读权限。
	viewer := &auth.User{
		Roles: []auth.Role{
			{
				Name: "viewer",
				Permissions: []auth.Permission{
					{Name: "alerts:read", Resource: "alerts", Action: "read"},
				},
			},
		},
	}
	if !viewer.HasPermission("alerts", "read") {
		t.Fatal("viewer should have alerts:read")
	}
	if viewer.HasPermission("alerts", "write") {
		t.Fatal("viewer should NOT have alerts:write")
	}
}

func TestUserRoleNames(t *testing.T) {
	user := &auth.User{
		Roles: []auth.Role{{Name: "admin"}, {Name: "operator"}},
	}
	names := user.RoleNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(names))
	}
}
