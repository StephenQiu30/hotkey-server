package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
)

// RegisterAuthRoutes registers the auth endpoints (register, login).
func RegisterAuthRoutes(r *gin.Engine, svc *auth.Service, jwtSecret string) {
	r.POST("/api/v1/auth/register", func(c *gin.Context) {
		var body struct {
			Email       string `json:"email" binding:"required,email"`
			Password    string `json:"password" binding:"required,min=8"`
			DisplayName string `json:"display_name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "invalid input")
			return
		}

		user, err := svc.Register(c.Request.Context(), auth.RegisterInput{
			Email:       body.Email,
			Password:    body.Password,
			DisplayName: body.DisplayName,
		})
		if err != nil {
			switch {
			case err == auth.ErrEmailExists:
				respondError(c, http.StatusConflict, "email already registered")
			case err == auth.ErrInvalidInput:
				respondError(c, http.StatusBadRequest, "invalid input")
			default:
				respondInternalError(c)
			}
			return
		}

		RespondCreated(c, UserResponse{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
		})
	})

	r.POST("/api/v1/auth/login", func(c *gin.Context) {
		var body struct {
			Email    string `json:"email" binding:"required,email"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "invalid input")
			return
		}

		user, err := svc.Login(c.Request.Context(), auth.LoginInput{
			Email:    body.Email,
			Password: body.Password,
		})
		if err != nil {
			switch {
			case err == auth.ErrInvalidCredentials:
				respondError(c, http.StatusUnauthorized, "invalid credentials")
			default:
				respondInternalError(c)
			}
			return
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":   user.ID,
			"email": user.Email,
			"exp":   time.Now().Add(24 * time.Hour).Unix(),
		})
		tokenStr, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			respondInternalError(c)
			return
		}

		RespondOK(c, LoginResponse{
			User: UserResponse{
				ID:          user.ID,
				Email:       user.Email,
				DisplayName: user.DisplayName,
			},
			Token: tokenStr,
		})
	})
}

// UserResponse is the JSON representation of a user.
type UserResponse struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// LoginResponse is the JSON representation of a login response.
type LoginResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}
