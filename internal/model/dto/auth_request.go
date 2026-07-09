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
