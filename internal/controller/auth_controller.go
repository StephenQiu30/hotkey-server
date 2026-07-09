package controller

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/convert"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func RegisterAuthRoutes(r *gin.Engine, svc *service.AuthService, jwtSecret string) {
	r.POST("/api/v1/auth/register", registerHandler(svc))
	r.POST("/api/v1/auth/login", loginHandler(svc, jwtSecret))
}

// registerHandler godoc
// @Summary Register a new user
// @ID register
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RegisterRequest true "Register payload"
// @Success 201 {object} UserResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 409 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/register [post]
func registerHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body RegisterRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "invalid input")
			return
		}

		user, err := svc.Register(c.Request.Context(), dto.RegisterInput{
			Email:       body.Email,
			Password:    body.Password,
			DisplayName: body.DisplayName,
		})
		if err != nil {
			switch {
			case err == service.AuthErrEmailExists:
				respondError(c, http.StatusConflict, "email already registered")
			case err == service.AuthErrInvalidInput:
				respondError(c, http.StatusBadRequest, "invalid input")
			default:
				respondInternalError(c)
			}
			return
		}

		RespondCreated(c, convert.UserDTOToVO(user))
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
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/login [post]
func loginHandler(svc *service.AuthService, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body LoginRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "invalid input")
			return
		}

		user, err := svc.Login(c.Request.Context(), dto.LoginInput{
			Email:    body.Email,
			Password: body.Password,
		})
		if err != nil {
			switch {
			case err == service.AuthErrInvalidCredentials:
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

		RespondOK(c, convert.LoginDTOToVO(user, tokenStr))
	}
}
