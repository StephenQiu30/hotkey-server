package domain

import (
	"errors"
	"net/mail"
	"strings"
	"time"
)

var ErrInvalidEmail = errors.New("invalid email")

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleAnalyst Role = "analyst"
	RoleEditor  Role = "editor"
	RoleViewer  Role = "viewer"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleAnalyst, RoleEditor, RoleViewer:
		return true
	default:
		return false
	}
}

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

func (s UserStatus) Valid() bool {
	return s == UserStatusActive || s == UserStatusDisabled
}

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	DisplayName  string
	Role         Role
	Status       UserStatus
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

func (u User) Active() bool {
	return u.Status == UserStatusActive && u.DeletedAt == nil
}

func NormalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(normalized)
	if err != nil || address.Address != normalized || !strings.Contains(normalized, "@") {
		return "", ErrInvalidEmail
	}
	return normalized, nil
}
