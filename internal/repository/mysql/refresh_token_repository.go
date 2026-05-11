package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"editorial-content-api/internal/domain"
	"gorm.io/gorm"
)

// ErrRefreshTokenNotFound is returned when a refresh token lookup misses.
var ErrRefreshTokenNotFound = errors.New("refresh token not found")

// RefreshTokenRepository persists refresh tokens in MySQL.
type RefreshTokenRepository struct {
	db *gorm.DB
}

// NewRefreshTokenRepository creates a MySQL-backed refresh token repository.
func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Create inserts a new refresh token row.
func (r *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	record := refreshTokenRecordFromDomain(token)
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return domain.RefreshToken{}, fmt.Errorf("create refresh token: %w", err)
	}

	return record.toDomain(), nil
}

// FindByHash returns the token row matching the given SHA-256 hash.
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash []byte) (domain.RefreshToken, error) {
	var record refreshTokenRecord
	err := r.db.WithContext(ctx).First(&record, "token_hash = ?", hash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.RefreshToken{}, ErrRefreshTokenNotFound
		}
		return domain.RefreshToken{}, fmt.Errorf("find refresh token: %w", err)
	}

	return record.toDomain(), nil
}

// Revoke marks a token as revoked, optionally recording the rotation successor.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, id string, replacedBy string, revokedAt time.Time) error {
	updates := map[string]any{
		"revoked_at": revokedAt,
	}
	if replacedBy != "" {
		updates["replaced_by"] = replacedBy
	}

	result := r.db.WithContext(ctx).Model(&refreshTokenRecord{}).
		Where("id = ? and revoked_at is null", id).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("revoke refresh token: %w", result.Error)
	}

	return nil
}

// RevokeAllForUser invalidates every active refresh token belonging to the user.
// Used when re-use of a revoked token is detected, when a user is disabled, or
// when the user changes their password.
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string, revokedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&refreshTokenRecord{}).
		Where("user_id = ? and revoked_at is null", userID).
		Update("revoked_at", revokedAt)
	if result.Error != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", result.Error)
	}

	return nil
}

// DeleteExpired removes refresh tokens whose expiration is in the past. It
// returns the number of rows deleted.
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("expires_at < ?", before).
		Delete(&refreshTokenRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired refresh tokens: %w", result.Error)
	}

	return result.RowsAffected, nil
}

type refreshTokenRecord struct {
	ID         string     `gorm:"column:id;type:varchar(32);primaryKey"`
	UserID     string     `gorm:"column:user_id;type:varchar(32);not null;index:refresh_tokens_user_idx"`
	TokenHash  []byte     `gorm:"column:token_hash;type:varbinary(32);not null;uniqueIndex:refresh_tokens_hash_idx"`
	UserAgent  *string    `gorm:"column:user_agent;type:varchar(255)"`
	IPAddress  *string    `gorm:"column:ip_address;type:varchar(64)"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;type:datetime(6);not null;index:refresh_tokens_expires_idx"`
	RevokedAt  *time.Time `gorm:"column:revoked_at;type:datetime(6)"`
	ReplacedBy *string    `gorm:"column:replaced_by;type:varchar(32)"`
	CreatedAt  time.Time  `gorm:"column:created_at;type:datetime(6);not null;autoCreateTime"`
}

func (refreshTokenRecord) TableName() string {
	return "refresh_tokens"
}

func refreshTokenRecordFromDomain(token domain.RefreshToken) refreshTokenRecord {
	return refreshTokenRecord{
		ID:         token.ID,
		UserID:     token.UserID,
		TokenHash:  token.TokenHash,
		UserAgent:  stringPointer(token.UserAgent),
		IPAddress:  stringPointer(token.IPAddress),
		ExpiresAt:  token.ExpiresAt,
		RevokedAt:  token.RevokedAt,
		ReplacedBy: stringPointer(token.ReplacedBy),
		CreatedAt:  token.CreatedAt,
	}
}

func (r refreshTokenRecord) toDomain() domain.RefreshToken {
	token := domain.RefreshToken{
		ID:        r.ID,
		UserID:    r.UserID,
		TokenHash: r.TokenHash,
		ExpiresAt: r.ExpiresAt,
		RevokedAt: r.RevokedAt,
		CreatedAt: r.CreatedAt,
	}
	if r.UserAgent != nil {
		token.UserAgent = *r.UserAgent
	}
	if r.IPAddress != nil {
		token.IPAddress = *r.IPAddress
	}
	if r.ReplacedBy != nil {
		token.ReplacedBy = *r.ReplacedBy
	}

	return token
}
