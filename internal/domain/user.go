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
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Role         UserRole   `json:"role"`
	IsActive     bool       `json:"isActive"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}
