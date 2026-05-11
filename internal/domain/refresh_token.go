package domain

import "time"

// RefreshToken represents a long-lived credential exchanged for short-lived
// access tokens. The raw token value is never stored; only its hash is kept.
type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  []byte
	UserAgent  string
	IPAddress  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy string
	CreatedAt  time.Time
}

// IsRevoked reports whether the token has been explicitly invalidated.
func (t RefreshToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

// IsExpired reports whether the token has passed its TTL.
func (t RefreshToken) IsExpired(now time.Time) bool {
	return !t.ExpiresAt.After(now)
}

// IsActive returns true only when the token is neither revoked nor expired.
func (t RefreshToken) IsActive(now time.Time) bool {
	return !t.IsRevoked() && !t.IsExpired(now)
}
