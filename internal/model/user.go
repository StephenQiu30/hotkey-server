package model

import "time"

// User represents a registered user account.
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

// RegisterInput is the input for user registration.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

// LoginInput is the input for user login.
type LoginInput struct {
	Email    string
	Password string
}
