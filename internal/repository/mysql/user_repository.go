package mysql

import (
	"context"
	"fmt"
	"time"

	"editorial-content-api/internal/domain"
	"gorm.io/gorm"
)

// UserRepository persists admin users in MySQL.
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a MySQL-backed user repository.
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByEmail returns an active or inactive user by email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	var record userRecord
	err := r.db.WithContext(ctx).First(&record, "email = ?", email).Error
	if err != nil {
		return domain.User{}, fmt.Errorf("find user by email: %w", err)
	}

	return record.toDomain(), nil
}

// UpdateLastLoginAt records a successful login timestamp.
func (r *UserRepository) UpdateLastLoginAt(ctx context.Context, id string, loggedInAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&userRecord{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": loggedInAt,
		"updated_at":    loggedInAt,
	})
	if result.Error != nil {
		return fmt.Errorf("update user last login: %w", result.Error)
	}

	return nil
}

type userRecord struct {
	ID           string          `gorm:"column:id;type:varchar(32);primaryKey"`
	Email        string          `gorm:"column:email;type:varchar(320);not null;uniqueIndex"`
	PasswordHash string          `gorm:"column:password_hash;type:varchar(255);not null"`
	Role         domain.UserRole `gorm:"column:role;type:varchar(20);not null;check:role in ('admin');index:users_role_active_idx,priority:1"`
	IsActive     bool            `gorm:"column:is_active;not null;default:true;index:users_role_active_idx,priority:2"`
	LastLoginAt  *time.Time      `gorm:"column:last_login_at;type:datetime(6)"`
	CreatedAt    time.Time       `gorm:"column:created_at;type:datetime(6);not null;autoCreateTime"`
	UpdatedAt    time.Time       `gorm:"column:updated_at;type:datetime(6);not null;autoUpdateTime"`
}

func (userRecord) TableName() string {
	return "users"
}

func (r userRecord) toDomain() domain.User {
	return domain.User{
		ID:           r.ID,
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Role:         r.Role,
		IsActive:     r.IsActive,
		LastLoginAt:  r.LastLoginAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
