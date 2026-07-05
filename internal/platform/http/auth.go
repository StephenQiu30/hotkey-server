package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
)

func RegisterAuthRoutes(r *gin.Engine, svc *auth.Service, jwtSecret string) {
	r.POST("/api/v1/auth/register", registerHandler(svc))
	r.POST("/api/v1/auth/login", loginHandler(svc, jwtSecret))
}

// UserData is the JSON representation of a user (nested inside ResponseBody.Data).
type UserData struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// LoginData is the JSON representation of a login response (nested inside ResponseBody.Data).
type LoginData struct {
	User  UserData `json:"user"`
	Token string   `json:"token"`
}

// registerHandler godoc
// @Summary Register a new user
// @ID register
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RegisterRequest true "Register payload"
// @Success 201 {object} UserResponse
// @Failure 400 {object} ErrorBody
// @Failure 409 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/auth/register [post]
func registerHandler(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body RegisterRequest
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

		RespondCreated(c, UserData{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName})
	}
}

// loginHandler godoc
// @Summary Login with email and password
// @ID login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "Login payload"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/auth/login [post]
func loginHandler(svc *auth.Service, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body LoginRequest
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

		RespondOK(c, LoginData{
			User:  UserData{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName},
			Token: tokenStr,
		})
	}
}
