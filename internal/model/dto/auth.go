package dto

import "time"

// User represents a registered user (domain model).
type User struct {
	ID                 int64
	Email              string
	PasswordHash       string
	DisplayName        string
	Status             string
	PlanType           string
	VerificationStatus string
	EmailVerifiedAt    *time.Time
	PasswordChangedAt  *time.Time
	LastLoginAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
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

// VerificationSendInput holds data for sending a verification code.
type VerificationSendInput struct {
	Email   string
	Purpose string
	IP      string
}

// VerificationConfirmInput holds data for confirming a verification code.
type VerificationConfirmInput struct {
	Email   string
	Purpose string
	Code    string
}

// TokenRefreshInput holds data for refreshing a session token.
type TokenRefreshInput struct {
	RefreshToken string
	FamilyHash   string
	UserID       int64
}

// PasswordResetInput holds data for resetting a password.
type PasswordResetInput struct {
	Email       string
	Code        string
	NewPassword string
}
