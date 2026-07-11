package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/convert"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

func RegisterAuthRoutes(r gin.IRouter, svc *service.AuthService, jwtSecret string) {
	r.POST("/api/v1/auth/register", registerHandler(svc))
	r.POST("/api/v1/auth/login", loginHandler(svc, jwtSecret))
}

// registerHandler godoc
// @Summary Register a new user
// @ID register
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.RegisterRequest true "Register payload"
// @Success 201 {object} UserResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 409 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/register [post]
func registerHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body dto.RegisterRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
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
				platformhttp.RespondError(c, enum.ErrorCodeConflict, "email already registered")
			case err == service.AuthErrInvalidInput:
				platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		platformhttp.RespondCreated(c, convert.UserDTOToVO(user))
	}
}

// loginHandler godoc
// @Summary Login with email and password
// @ID login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.LoginRequest true "Login payload"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/login [post]
func loginHandler(svc *service.AuthService, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body dto.LoginRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			return
		}

		user, err := svc.Login(c.Request.Context(), dto.LoginInput{
			Email:    body.Email,
			Password: body.Password,
		})
		if err != nil {
			switch {
			case err == service.AuthErrInvalidCredentials:
				platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "invalid credentials")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		// Sign a typed JWT using the security package for consistent claims.
		claims := security.AccessClaims{
			SessionID: 0, // TODO: implement session management
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: strconv.FormatInt(user.ID, 10),
			},
		}
		tokenStr, err := security.SignAccessToken(claims, jwtSecret)
		if err != nil {
			platformhttp.RespondInternalError(c)
			return
		}

		platformhttp.RespondOK(c, convert.LoginDTOToVO(user, tokenStr))
	}
}
