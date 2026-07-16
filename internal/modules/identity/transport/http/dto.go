package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
)

// IdentityResult mirrors the platform Result envelope only for Swagger's
// source parser, which cannot resolve an imported generic type annotation.
// Runtime responses are always written by internal/platform/http helpers.
type IdentityResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// RequestVerificationRequest intentionally carries only the public email and
// flow purpose. The submitted code is never echoed in any response DTO.
type RequestVerificationRequest struct {
	Email   string `json:"email" binding:"required" example:"reader@example.test"`
	Purpose string `json:"purpose" binding:"required,oneof=registration password_reset" example:"registration"`
}

type ConfirmVerificationRequest struct {
	Email   string `json:"email" binding:"required" example:"reader@example.test"`
	Purpose string `json:"purpose" binding:"required,oneof=registration password_reset" example:"registration"`
	Code    string `json:"code" binding:"required" example:"123456"`
}

// ConfirmVerificationResponse contains the deliberately short-lived,
// single-purpose ticket required by the next registration/reset request. It
// never contains the submitted code or any refresh credential.
type ConfirmVerificationResponse struct {
	VerificationTicket string `json:"verification_ticket" example:"opaque-single-use-ticket"`
}

type RegistrationRequest struct {
	VerificationTicket string `json:"verification_ticket" binding:"required"`
	Password           string `json:"password" binding:"required"`
	DisplayName        string `json:"display_name" binding:"required" example:"Reader"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required" example:"reader@example.test"`
	Password string `json:"password" binding:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

type ConfirmPasswordResetRequest struct {
	VerificationTicket string `json:"verification_ticket" binding:"required"`
	Password           string `json:"password" binding:"required"`
}

type UpdateUserRequest struct {
	Role   *string `json:"role,omitempty" enums:"admin,editor,viewer"`
	Status *string `json:"status,omitempty" enums:"active,disabled"`
}

// UserResponse is the only user shape exposed at the HTTP boundary. In
// particular, domain.PasswordHash and repository state never cross this DTO.
type UserResponse struct {
	ID          int64      `json:"id" example:"3"`
	Email       string     `json:"email" example:"reader@example.test"`
	DisplayName string     `json:"display_name" example:"Reader"`
	Role        string     `json:"role" enums:"admin,editor,viewer"`
	Status      string     `json:"status" enums:"active,disabled"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type AuthenticationResponse struct {
	AccessToken string       `json:"access_token" example:"signed-access-token"`
	User        UserResponse `json:"user"`
}

// EmptyResponse documents successful operations whose actual Result data is
// null. It has no fields and is never populated with domain state.
type EmptyResponse struct{}

func userResponse(user domain.User) UserResponse {
	return UserResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        string(user.Role),
		Status:      string(user.Status),
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		DeletedAt:   user.DeletedAt,
	}
}

func userResponses(users []domain.User) []UserResponse {
	responses := make([]UserResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, userResponse(user))
	}
	return responses
}
