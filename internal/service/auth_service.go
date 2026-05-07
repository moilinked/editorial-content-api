package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"editorial-content-api/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidCredentials is returned when login credentials are invalid.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnauthorized is returned when an access token is missing or invalid.
	ErrUnauthorized = errors.New("unauthorized")
)

// UserRepository defines user persistence operations required by AuthService.
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	UpdateLastLoginAt(ctx context.Context, id string, loggedInAt time.Time) error
}

// AuthConfig contains token signing settings.
type AuthConfig struct {
	JWTSecret      string
	JWTIssuer      string
	AccessTokenTTL time.Duration
}

// LoginInput contains admin login credentials.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResult contains the issued access token and authenticated user.
type LoginResult struct {
	AccessToken string      `json:"accessToken"`
	TokenType   string      `json:"tokenType"`
	ExpiresAt   time.Time   `json:"expiresAt"`
	User        domain.User `json:"user"`
}

// AuthenticatedUser represents the trusted identity extracted from JWT claims.
type AuthenticatedUser struct {
	ID    string          `json:"id"`
	Email string          `json:"email"`
	Role  domain.UserRole `json:"role"`
}

// AuthService handles admin login and JWT verification.
type AuthService struct {
	repo           UserRepository
	jwtSecret      []byte
	jwtIssuer      string
	accessTokenTTL time.Duration
}

// NewAuthService creates an AuthService.
func NewAuthService(repo UserRepository, cfg AuthConfig) *AuthService {
	return &AuthService{
		repo:           repo,
		jwtSecret:      []byte(cfg.JWTSecret),
		jwtIssuer:      cfg.JWTIssuer,
		accessTokenTTL: cfg.AccessTokenTTL,
	}
}

// Login validates credentials and returns a signed access token.
func (s *AuthService) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || input.Password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if !user.IsActive || user.Role != domain.UserRoleAdmin {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.accessTokenTTL)
	token, err := s.signToken(user, now, expiresAt)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.repo.UpdateLastLoginAt(ctx, user.ID, now); err != nil {
		return LoginResult{}, err
	}
	user.LastLoginAt = &now

	return LoginResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        user,
	}, nil
}

// VerifyAccessToken validates a JWT access token and returns the authenticated user.
func (s *AuthService) VerifyAccessToken(tokenString string) (AuthenticatedUser, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return AuthenticatedUser{}, ErrUnauthorized
	}

	claims := &accessTokenClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
		}

		return s.jwtSecret, nil
	}, jwt.WithIssuer(s.jwtIssuer))
	if err != nil || !token.Valid {
		return AuthenticatedUser{}, ErrUnauthorized
	}
	if claims.Subject == "" || claims.Email == "" || claims.Role != domain.UserRoleAdmin {
		return AuthenticatedUser{}, ErrUnauthorized
	}

	return AuthenticatedUser{
		ID:    claims.Subject,
		Email: claims.Email,
		Role:  claims.Role,
	}, nil
}

func (s *AuthService) signToken(user domain.User, issuedAt time.Time, expiresAt time.Time) (string, error) {
	claims := accessTokenClaims{
		Email: user.Email,
		Role:  user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    s.jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return token, nil
}

type accessTokenClaims struct {
	Email string          `json:"email"`
	Role  domain.UserRole `json:"role"`
	jwt.RegisteredClaims
}
