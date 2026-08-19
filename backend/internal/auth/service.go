package auth

import (
	"context"
	"errors"
	"time"
)

// Service 认证服务。
type Service struct {
	repo      *Repository
	jwtConfig JWTConfig
}

// NewService 创建认证服务。
func NewService(repo *Repository, jwtSecret string, jwtExpiration time.Duration) *Service {
	return &Service{
		repo: repo,
		jwtConfig: JWTConfig{
			Secret:     jwtSecret,
			Expiration: jwtExpiration,
		},
	}
}

// LoginRequest 登录请求。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应。
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"` // Bearer
	ExpiresIn   int64  `json:"expires_in"` // 秒
	User        *User  `json:"user"`
}

// Login 用户登录。
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if req.Username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}

	user, err := s.repo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("用户名或密码错误")
	}
	if user.Status != "active" {
		return nil, errors.New("用户已被禁用")
	}

	if !CheckPassword(req.Password, user.PasswordHash) {
		return nil, errors.New("用户名或密码错误")
	}

	// 生成 Token。
	token, err := GenerateToken(user, s.jwtConfig)
	if err != nil {
		return nil, err
	}

	// 更新最后登录时间。
	_ = s.repo.UpdateLastLogin(ctx, user.ID)

	expiration := s.jwtConfig.Expiration
	if expiration <= 0 {
		expiration = 24 * time.Hour
	}

	return &LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(expiration.Seconds()),
		User:        user,
	}, nil
}

// ValidateToken 验证 Token 并返回对应用户。
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (*User, error) {
	claims, err := ParseToken(tokenString, s.jwtConfig.Secret)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("用户不存在")
	}
	if user.Status != "active" {
		return nil, errors.New("用户已被禁用")
	}
	return user, nil
}
