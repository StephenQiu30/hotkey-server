package dto

import "time"

// User represents a registered user (domain model).
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
