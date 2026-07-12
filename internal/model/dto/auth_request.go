package dto

// RegisterRequest is the request body for POST /api/v1/auth/register.
type RegisterRequest struct {
	Email       string `json:"email" example:"user@example.com"`
	Password    string `json:"password" example:"Passw0rd!"`
	DisplayName string `json:"display_name" example:"Stephen"`
}

// LoginRequest is the request body for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"Passw0rd!"`
}

// VerificationSendRequest is the request body for sending a verification code.
type VerificationSendRequest struct {
	Email   string `json:"email" binding:"required,email"`
	Purpose string `json:"purpose" binding:"required,oneof=register reset_password"`
}

// VerificationConfirmRequest is the request body for confirming a verification code.
type VerificationConfirmRequest struct {
	Email   string `json:"email" binding:"required,email"`
	Code    string `json:"code" binding:"required,len=6"`
	Purpose string `json:"purpose" binding:"required,oneof=register reset_password"`
}

// EmailRegisterRequest is the request body for email-based registration (after verification).
type EmailRegisterRequest struct {
	VerificationTicket string `json:"verification_ticket" binding:"required"`
	Password           string `json:"password" binding:"required,min=8,max=128"`
	DisplayName        string `json:"display_name" binding:"required,max=80"`
}

// EmailLoginRequest is the request body for email-based login.
type EmailLoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// PasswordResetRequest is the request body for resetting a password.
type PasswordResetRequest struct {
	ResetToken  string `json:"reset_token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}

// TokenRefreshRequest is the request body for refreshing a session token.
type TokenRefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest is the request body for logging out a session.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
