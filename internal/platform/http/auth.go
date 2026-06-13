package http

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
)

// RegisterAuthRoutes registers the auth endpoints (register, login).
// Auth endpoints do NOT require JWT authentication.
func RegisterAuthRoutes(api huma.API, svc *auth.Service, jwtSecret string) {
	huma.Register(api, huma.Operation{
		OperationID:  "register",
		Method:       http.MethodPost,
		Path:         "/api/v1/auth/register",
		Summary:      "Register a new user",
		Description:  "Creates a new user account with email, password, and display name.",
		Tags:         []string{"auth"},
		DefaultStatus: http.StatusCreated,
		Errors:       []int{400, 409, 500},
	}, func(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
		user, err := svc.Register(ctx, auth.RegisterInput{
			Email:       input.Body.Email,
			Password:    input.Body.Password,
			DisplayName: input.Body.DisplayName,
		})
		if err != nil {
			switch {
			case err == auth.ErrEmailExists:
				return nil, huma.Error409Conflict("email already registered")
			case err == auth.ErrInvalidInput:
				return nil, huma.Error400BadRequest("invalid input")
			default:
				return nil, huma.Error500InternalServerError("internal error")
			}
		}

		return &RegisterOutput{
			Body: UserResponse{
				ID:          user.ID,
				Email:       user.Email,
				DisplayName: user.DisplayName,
			},
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/login",
		Summary:     "Login with email and password",
		Description: "Authenticates a user and returns a JWT token.",
		Tags:        []string{"auth"},
		Errors:      []int{400, 401, 500},
	}, func(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
		user, err := svc.Login(ctx, auth.LoginInput{
			Email:    input.Body.Email,
			Password: input.Body.Password,
		})
		if err != nil {
			switch {
			case err == auth.ErrInvalidCredentials:
				return nil, huma.Error401Unauthorized("invalid credentials")
			default:
				return nil, huma.Error500InternalServerError("internal error")
			}
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":   user.ID,
			"email": user.Email,
			"exp":   time.Now().Add(24 * time.Hour).Unix(),
		})
		tokenStr, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to sign token")
		}

		return &LoginOutput{
			Body: LoginResponse{
				User: UserResponse{
					ID:          user.ID,
					Email:       user.Email,
					DisplayName: user.DisplayName,
				},
				Token: tokenStr,
			},
		}, nil
	})
}

// --- Input / Output types ---

// RegisterInput is the request body for POST /api/v1/auth/register.
type RegisterInput struct {
	Body struct {
		Email       string `json:"email" validate:"required,email" doc:"User email address"`
		Password    string `json:"password" validate:"required,min=8" doc:"User password (min 8 chars)"`
		DisplayName string `json:"display_name" validate:"required,min=1" doc:"Display name"`
	}
}

// RegisterOutput is the response for POST /api/v1/auth/register.
type RegisterOutput struct {
	Body UserResponse
}

// LoginInput is the request body for POST /api/v1/auth/login.
type LoginInput struct {
	Body struct {
		Email    string `json:"email" validate:"required,email" doc:"User email address"`
		Password string `json:"password" validate:"required" doc:"User password"`
	}
}

// LoginOutput is the response for POST /api/v1/auth/login.
type LoginOutput struct {
	Body LoginResponse
}

// UserResponse is the JSON representation of a user.
type UserResponse struct {
	ID          int64  `json:"id" doc:"User ID"`
	Email       string `json:"email" doc:"User email"`
	DisplayName string `json:"display_name" doc:"User display name"`
}

// LoginResponse is the JSON representation of a login response.
type LoginResponse struct {
	User  UserResponse `json:"user" doc:"Authenticated user"`
	Token string       `json:"token" doc:"JWT access token"`
}
