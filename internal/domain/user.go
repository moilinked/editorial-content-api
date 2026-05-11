package domain

import "time"

// UserRole describes what an authenticated user can access.
type UserRole string

const (
	// UserRoleAdmin can access the editorial admin API.
	UserRoleAdmin UserRole = "admin"
)

// User stores administrator identity and password metadata.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         UserRole
	IsActive     bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
