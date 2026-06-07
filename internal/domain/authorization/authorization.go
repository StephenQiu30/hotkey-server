package authorization

import (
	"errors"
	"time"
)

type Platform string

const (
	PlatformGitHub  Platform = "github"
	PlatformWeChat  Platform = "wechat"
	PlatformRSS     Platform = "rss"
	PlatformCustom  Platform = "custom"
)

type Status string

const (
	StatusConnected Status = "connected"
	StatusExpired   Status = "expired"
	StatusRevoked   Status = "revoked"
)

var (
	ErrNotFound        = errors.New("authorization not found")
	ErrInvalidPlatform = errors.New("invalid platform")
	ErrAlreadyRevoked  = errors.New("authorization already revoked")
	ErrUniqueViolation = errors.New("authorization already exists for this platform")
)

type Authorization struct {
	ID             string
	UserID         string
	Platform       Platform
	PlatformUserID string
	DisplayName    string
	AccessTokenEnc string // encrypted access token
	RefreshTokenEnc string // encrypted refresh token (optional)
	Status         Status
	ConnectedAt    time.Time
	LastCheckedAt  time.Time
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (a Authorization) IsExpired(now time.Time) bool {
	if a.ExpiresAt == nil {
		return false
	}
	return !a.ExpiresAt.After(now)
}

func (a Authorization) IsRevoked() bool {
	return a.Status == StatusRevoked
}

func ValidPlatform(p Platform) bool {
	switch p {
	case PlatformGitHub, PlatformWeChat, PlatformRSS, PlatformCustom:
		return true
	}
	return false
}
