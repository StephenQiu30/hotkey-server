package domain

import "time"

const (
	AccessTokenLifetime     = 15 * time.Minute
	RefreshSessionLifetime  = 7 * 24 * time.Hour
	SessionAbsoluteLifetime = 30 * 24 * time.Hour
)

type Session struct {
	ID                int64
	UserID            int64
	FamilyID          string
	AbsoluteExpiresAt time.Time
	LastSeenAt        time.Time
	RevokedAt         *time.Time
	RevokeReason      string
	CreatedAt         time.Time
}

func NewSession(userID int64, familyID string, now time.Time) Session {
	now = now.UTC()
	return Session{
		UserID:            userID,
		FamilyID:          familyID,
		AbsoluteExpiresAt: now.Add(SessionAbsoluteLifetime),
		LastSeenAt:        now,
		CreatedAt:         now,
	}
}

func (s Session) RefreshExpiry(now time.Time) time.Time {
	expiresAt := now.UTC().Add(RefreshSessionLifetime)
	if expiresAt.After(s.AbsoluteExpiresAt) {
		return s.AbsoluteExpiresAt
	}
	return expiresAt
}

type RefreshToken struct {
	ID        int64
	SessionID int64
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

type AccessTokenClaims struct {
	UserID    int64
	SessionID int64
	TokenID   string
	IssuedAt  time.Time
	NotBefore time.Time
	ExpiresAt time.Time
}

type Subject struct {
	UserID    int64
	SessionID int64
	Role      Role
}
