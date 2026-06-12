package auth

import (
	"errors"
	"time"
)

// Sentinel errors for auth operations.
var (
	ErrEmailExists    = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidInput   = errors.New("invalid input")
)

// User represents a registered user.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	DisplayName  string
	Status       string
	PlanType     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RegisterInput holds data for user registration.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

// LoginInput holds data for user login.
type LoginInput struct {
	Email    string
	Password string
}
