package user

import (
	"errors"
	"strings"
	"time"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	Status       Status
	Timezone     string
	DailySendAt  string
	WeChatOpenID string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var ErrInvalidEmail = errors.New("invalid email")

func NormalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || !strings.Contains(normalized, "@") {
		return "", ErrInvalidEmail
	}
	return normalized, nil
}

func NewEmailUser(id string, email string, passwordHash string, now time.Time) (User, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	return User{
		ID:           id,
		Email:        normalized,
		PasswordHash: passwordHash,
		Role:         RoleUser,
		Status:       StatusActive,
		Timezone:     "Asia/Shanghai",
		DailySendAt:  "08:30",
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}
