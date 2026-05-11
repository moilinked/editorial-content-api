package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
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
	// ErrInvalidRefreshToken is returned when a refresh token is missing,
	// malformed, expired, revoked, or already rotated.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

// UserRepository defines user persistence operations required by AuthService.
type UserRepository interface {
	FindByID(ctx context.Context, id string) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	UpdateLastLoginAt(ctx context.Context, id string, loggedInAt time.Time) error
}

// RefreshTokenRepository defines refresh token persistence operations.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error)
	FindByHash(ctx context.Context, hash []byte) (domain.RefreshToken, error)
	Revoke(ctx context.Context, id string, replacedBy string, revokedAt time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, revokedAt time.Time) error
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// AuthConfig contains token signing settings.
type AuthConfig struct {
	JWTSecret       string
	JWTIssuer       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// ClientMetadata captures the originating client of an auth request and is
// stored alongside refresh tokens for audit and anomaly detection.
type ClientMetadata struct {
	UserAgent string
	IPAddress string
}

// LoginInput contains admin login credentials.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResult contains the access token plus the rotated refresh token. The
// raw refresh token is returned exactly once so transport layers can place it
// in an HttpOnly cookie; it is never persisted in plaintext.
type LoginResult struct {
	AccessToken      string
	TokenType        string
	ExpiresAt        time.Time
	User             domain.User
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// AuthenticatedUser represents the trusted identity extracted from JWT claims.
type AuthenticatedUser struct {
	ID    string          `json:"id"`
	Email string          `json:"email"`
	Role  domain.UserRole `json:"role"`
}

// AuthService handles admin login, refresh token rotation and JWT verification.
type AuthService struct {
	users           UserRepository
	refreshTokens   RefreshTokenRepository
	jwtSecret       []byte
	jwtIssuer       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	logger          *slog.Logger
}

// NewAuthService creates an AuthService. logger may be nil.
func NewAuthService(
	users UserRepository,
	refreshTokens RefreshTokenRepository,
	cfg AuthConfig,
	logger *slog.Logger,
) *AuthService {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthService{
		users:           users,
		refreshTokens:   refreshTokens,
		jwtSecret:       []byte(cfg.JWTSecret),
		jwtIssuer:       cfg.JWTIssuer,
		accessTokenTTL:  cfg.AccessTokenTTL,
		refreshTokenTTL: cfg.RefreshTokenTTL,
		logger:          logger,
	}
}

// Login validates credentials and issues both an access token and a refresh
// token. Client metadata is stored with the refresh token for audit.
func (s *AuthService) Login(ctx context.Context, input LoginInput, client ClientMetadata) (LoginResult, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || input.Password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	user, err := s.users.FindByEmail(ctx, email)
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
	if err := s.users.UpdateLastLoginAt(ctx, user.ID, now); err != nil {
		return LoginResult{}, err
	}
	user.LastLoginAt = &now

	return s.issueTokens(ctx, user, client, now)
}

// Refresh validates a refresh token, rotates it (revoking the previous one),
// and returns a freshly signed access token plus the new refresh token. If the
// supplied token has already been revoked, every refresh token for the user is
// invalidated as a precaution against replay.
func (s *AuthService) Refresh(ctx context.Context, rawToken string, client ClientMetadata) (LoginResult, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return LoginResult{}, ErrInvalidRefreshToken
	}

	hash := hashRefreshToken(rawToken)
	stored, err := s.refreshTokens.FindByHash(ctx, hash)
	if err != nil {
		return LoginResult{}, ErrInvalidRefreshToken
	}

	now := time.Now().UTC()

	if stored.IsRevoked() {
		// A revoked token must never be accepted. Assume replay and invalidate
		// the rest of the user's sessions.
		if err := s.refreshTokens.RevokeAllForUser(ctx, stored.UserID, now); err != nil {
			s.logger.Warn("revoke all refresh tokens after replay", "user_id", stored.UserID, "error", err)
		}
		s.logger.Warn("refresh token replay detected", "user_id", stored.UserID, "token_id", stored.ID)
		return LoginResult{}, ErrInvalidRefreshToken
	}
	if stored.IsExpired(now) {
		return LoginResult{}, ErrInvalidRefreshToken
	}

	user, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil {
		return LoginResult{}, ErrInvalidRefreshToken
	}
	if !user.IsActive || user.Role != domain.UserRoleAdmin {
		_ = s.refreshTokens.RevokeAllForUser(ctx, user.ID, now)
		return LoginResult{}, ErrInvalidRefreshToken
	}

	issued, err := s.issueTokens(ctx, user, client, now)
	if err != nil {
		return LoginResult{}, err
	}

	if err := s.refreshTokens.Revoke(ctx, stored.ID, refreshTokenIDFromRaw(issued.RefreshToken), now); err != nil {
		s.logger.Warn("revoke rotated refresh token", "token_id", stored.ID, "error", err)
	}

	return issued, nil
}

// Logout revokes the supplied refresh token. Unknown or malformed tokens are
// treated as already revoked so callers can safely clear client cookies.
func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil
	}

	hash := hashRefreshToken(rawToken)
	stored, err := s.refreshTokens.FindByHash(ctx, hash)
	if err != nil {
		return nil
	}
	if stored.IsRevoked() {
		return nil
	}

	return s.refreshTokens.Revoke(ctx, stored.ID, "", time.Now().UTC())
}

// CleanupExpiredRefreshTokens deletes refresh tokens whose expiration has
// passed. Intended to be called from a periodic background job.
func (s *AuthService) CleanupExpiredRefreshTokens(ctx context.Context) (int64, error) {
	return s.refreshTokens.DeleteExpired(ctx, time.Now().UTC())
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

func (s *AuthService) issueTokens(
	ctx context.Context,
	user domain.User,
	client ClientMetadata,
	now time.Time,
) (LoginResult, error) {
	accessExpires := now.Add(s.accessTokenTTL)
	accessToken, err := s.signAccessToken(user, now, accessExpires)
	if err != nil {
		return LoginResult{}, err
	}

	rawRefresh, err := newRefreshTokenSecret()
	if err != nil {
		return LoginResult{}, err
	}

	refreshExpires := now.Add(s.refreshTokenTTL)
	stored := domain.RefreshToken{
		ID:        refreshTokenIDFromRaw(rawRefresh),
		UserID:    user.ID,
		TokenHash: hashRefreshToken(rawRefresh),
		UserAgent: truncate(client.UserAgent, 255),
		IPAddress: truncate(client.IPAddress, 64),
		ExpiresAt: refreshExpires,
		CreatedAt: now,
	}
	if _, err := s.refreshTokens.Create(ctx, stored); err != nil {
		return LoginResult{}, fmt.Errorf("persist refresh token: %w", err)
	}

	return LoginResult{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresAt:        accessExpires,
		User:             user,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: refreshExpires,
	}, nil
}

func (s *AuthService) signAccessToken(user domain.User, issuedAt time.Time, expiresAt time.Time) (string, error) {
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

// newRefreshTokenSecret returns a 32-byte cryptographically random secret
// encoded as URL-safe base64 (no padding). The encoded form is what we hand
// out to clients; only its SHA-256 hash is stored server-side.
func newRefreshTokenSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hashRefreshToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// refreshTokenIDFromRaw derives a stable, non-secret short ID from the token
// hash. Using it lets us reference tokens in rotation chains and logs without
// exposing the secret itself.
func refreshTokenIDFromRaw(raw string) string {
	hash := hashRefreshToken(raw)
	return fmt.Sprintf("%x", hash[:16])
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
