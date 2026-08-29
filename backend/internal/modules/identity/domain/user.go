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

type UserListStatus string

const (
	UserListStatusActive   UserListStatus = "active"
	UserListStatusDisabled UserListStatus = "disabled"
	UserListStatusDeleted  UserListStatus = "deleted"
)

func (status UserListStatus) Valid() bool {
	return status == "" || status == UserListStatusActive || status == UserListStatusDisabled || status == UserListStatusDeleted
}

// UserListQuery fixes the public administration list to an immutable ID
// traversal. Search and lifecycle filters are bound into the signed cursor by
// the repository rather than being applied to a partially loaded UI page.
type UserListQuery struct {
	Cursor string
	Limit  int
	Search string
	Role   *Role
	Status UserListStatus
}

type UserPage struct {
	Items      []User
	NextCursor string
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
