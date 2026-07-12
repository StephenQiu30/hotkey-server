package controller

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/convert"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
)

// ---------------------------------------------------------------------------
// Registration — supports both legacy (email) and ticket-based flows.
// ---------------------------------------------------------------------------

// registerHandler godoc
// @Summary Register a new user
// @ID register
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.RegisterRequest true "Register payload (legacy) or VerificationTicket payload"
// @Success 201 {object} UserResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 409 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/register [post]
func registerHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid request body")
			return
		}

		// Detect whether the body contains a verification_ticket (new flow)
		// or an email field (legacy flow).
		var raw map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid JSON")
			return
		}

		if ticket, ok := raw["verification_ticket"].(string); ok && ticket != "" {
			// ——— New ticket-based registration ———
			var body dto.EmailRegisterRequest
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
				return
			}

			result, err := svc.RegisterVerified(c.Request.Context(), body.VerificationTicket, body.Password, body.DisplayName, c.ClientIP(), c.GetHeader("User-Agent"))
			if err != nil {
				switch {
				case err == service.AuthErrEmailExists:
					platformhttp.RespondError(c, enum.ErrorCodeConflict, "email already registered")
				case err == service.AuthErrInvalidInput:
					platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
				case err == service.VerificationErrTicketNotFound:
					platformhttp.RespondError(c, enum.ErrorCodeNotFound, "ticket not found")
				case err == service.VerificationErrTicketClaimed:
					platformhttp.RespondError(c, enum.ErrorCodeConflict, "ticket already claimed")
				default:
					platformhttp.RespondInternalError(c)
				}
				return
			}

			setRefreshCookie(c, result.Tokens.RefreshToken, result.Tokens.RefreshExpiresAt)
			platformhttp.RespondCreated(c, convert.AuthResultToLoginVO(result))
		} else {
			// ——— Legacy direct registration ———
			var body dto.RegisterRequest
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
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
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

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

		result, err := svc.Login(c.Request.Context(), body.Email, body.Password, c.ClientIP(), c.GetHeader("User-Agent"))
		if err != nil {
			switch {
			case err == service.AuthErrInvalidCredentials:
				platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "invalid credentials")
			case err == service.AuthErrAccountDisabled:
				platformhttp.RespondError(c, enum.ErrorCodeForbidden, "account disabled")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		// Set refresh token as httpOnly cookie.
		if result.Tokens != nil {
			setRefreshCookie(c, result.Tokens.RefreshToken, result.Tokens.RefreshExpiresAt)

			platformhttp.RespondOK(c, convert.AuthResultToLoginVO(result))
			return
		}

		// Fallback: session-less mode (legacy wiring).
		claims := security.AccessClaims{
			RegisteredClaims: security.AccessClaims{}.RegisteredClaims,
		}
		claims.Subject = strconv.FormatInt(result.User.ID, 10)
		tokenStr, err := security.SignAccessToken(claims, jwtSecret)
		if err != nil {
			platformhttp.RespondInternalError(c)
			return
		}

		platformhttp.RespondOK(c, convert.LoginDTOToVO(result.User, tokenStr))
	}
}

// ---------------------------------------------------------------------------
// Token refresh
// ---------------------------------------------------------------------------

// refreshTokenHandler godoc
// @Summary Refresh access token
// @ID refresh-token
// @Tags auth
// @Produce json
// @Success 200 {object} AuthTokenResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/token/refresh [post]
func refreshTokenHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookieValue, err := c.Cookie(refreshCookieName)
		if err != nil || cookieValue == "" {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "missing refresh token")
			return
		}

		sessionID, refreshToken, ok := parseRefreshCookie(cookieValue)
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid refresh token")
			return
		}

		tokens, err := svc.RefreshSession(c.Request.Context(), sessionID, refreshToken)
		if err != nil {
			switch {
			case err == service.ErrSessionNotFound || err == service.ErrSessionRevoked:
				platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "session invalid")
			case err == service.ErrSessionExpired:
				platformhttp.RespondError(c, enum.ErrorCodeTokenExpired, "session expired")
			case err == service.ErrTokenReused:
				platformhttp.RespondError(c, enum.ErrorCodeTokenReused, "token reuse detected")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		setRefreshCookie(c, tokens.RefreshToken, tokens.RefreshExpiresAt)
		platformhttp.RespondOK(c, convert.TokensToAuthTokenData(tokens))
	}
}

// ---------------------------------------------------------------------------
// Logout
// ---------------------------------------------------------------------------

// logoutHandler godoc
// @Summary Logout and revoke session
// @ID logout
// @Tags auth
// @Produce json
// @Success 200 {object} platformhttp.ErrorBody
// @Failure 400 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/logout [post]
func logoutHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookieValue, err := c.Cookie(refreshCookieName)
		if err != nil || cookieValue == "" {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "missing refresh token")
			return
		}

		sessionID, _, ok := parseRefreshCookie(cookieValue)
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid refresh token")
			return
		}

		if err := svc.LogoutSession(c.Request.Context(), sessionID); err != nil {
			if err == service.ErrSessionNotFound {
				platformhttp.RespondError(c, enum.ErrorCodeNotFound, "session not found")
				return
			}
			platformhttp.RespondInternalError(c)
			return
		}

		clearRefreshCookie(c)
		platformhttp.RespondOK(c, gin.H{"success": true})
	}
}

// ---------------------------------------------------------------------------
// Current user profile (protected)
// ---------------------------------------------------------------------------

// meHandler godoc
// @Summary Get current user profile
// @ID me
// @Tags auth
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} AuthenticatedUserResponse
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/me [get]
func meHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := runtime.UserIDFromContext(c.Request.Context())
		if id == 0 {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "not authenticated")
			return
		}

		user, err := svc.CurrentUser(c.Request.Context(), id)
		if err != nil || user == nil {
			platformhttp.RespondError(c, enum.ErrorCodeNotFound, "user not found")
			return
		}

		platformhttp.RespondOK(c, convert.UserDTOToAuthenticatedUserVO(*user))
	}
}

// ---------------------------------------------------------------------------
// Verification code — send
// ---------------------------------------------------------------------------

// sendVerificationHandler godoc
// @Summary Send a verification code
// @ID send-verification
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.VerificationSendRequest true "Verification send payload"
// @Success 200 {object} VerificationSendResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 429 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/verifications [post]
func sendVerificationHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body dto.VerificationSendRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			return
		}

		err := svc.SendVerificationCode(c.Request.Context(), dto.VerificationSendInput{
			Email:   body.Email,
			Purpose: body.Purpose,
			IP:      c.ClientIP(),
		})
		if err != nil {
			switch {
			case err == service.VerificationErrLocked:
				platformhttp.RespondError(c, enum.ErrorCodeRateLimited, "resend locked, try again later")
			case err == service.VerificationErrSendLimit:
				platformhttp.RespondError(c, enum.ErrorCodeRateLimited, "send limit exceeded")
			case err == service.VerificationErrIPLimit:
				platformhttp.RespondError(c, enum.ErrorCodeRateLimited, "IP send limit exceeded")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		platformhttp.RespondOK(c, gin.H{"email": body.Email, "message": "verification code sent"})
	}
}

// ---------------------------------------------------------------------------
// Verification code — confirm
// ---------------------------------------------------------------------------

// confirmVerificationHandler godoc
// @Summary Confirm a verification code
// @ID confirm-verification
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.VerificationConfirmRequest true "Verification confirm payload"
// @Success 200 {object} VerificationTicketResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/verifications/confirm [post]
func confirmVerificationHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body dto.VerificationConfirmRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			return
		}

		ticket, err := svc.ConfirmCode(c.Request.Context(), dto.VerificationConfirmInput{
			Email:   body.Email,
			Code:    body.Code,
			Purpose: body.Purpose,
		})
		if err != nil {
			switch {
			case err == service.VerificationErrNotFound:
				platformhttp.RespondError(c, enum.ErrorCodeNotFound, "code not found")
			case err == service.VerificationErrInvalidCode:
				platformhttp.RespondError(c, enum.ErrorCodeInvalidVerificationCode, "invalid verification code")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		platformhttp.RespondOK(c, gin.H{"ticket": ticket})
	}
}

// ---------------------------------------------------------------------------
// Password reset
// ---------------------------------------------------------------------------

// resetPasswordHandler godoc
// @Summary Reset password with verification ticket
// @ID reset-password
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.PasswordResetRequest true "Password reset payload"
// @Success 200 {object} platformhttp.ErrorBody
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/auth/password/reset [post]
func resetPasswordHandler(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body dto.PasswordResetRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			return
		}

		err := svc.ResetPassword(c.Request.Context(), body.ResetToken, body.NewPassword)
		if err != nil {
			switch {
			case err == service.AuthErrInvalidInput:
				platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			case err == service.VerificationErrTicketNotFound:
				platformhttp.RespondError(c, enum.ErrorCodeNotFound, "ticket not found")
			case err == service.VerificationErrTicketClaimed:
				platformhttp.RespondError(c, enum.ErrorCodeConflict, "ticket already claimed")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}

		platformhttp.RespondOK(c, gin.H{"success": true})
	}
}

// ---------------------------------------------------------------------------
// Cookie helpers
// ---------------------------------------------------------------------------

// setRefreshCookie writes the refresh token as an httpOnly secure cookie.
func setRefreshCookie(c *gin.Context, refreshToken string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetCookie(refreshCookieName, refreshToken, maxAge, refreshCookiePath, "", true, true)
}

// clearRefreshCookie removes the refresh token cookie.
func clearRefreshCookie(c *gin.Context) {
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", true, true)
}

// parseRefreshCookie extracts the session ID and raw refresh token from a
// cookie value formatted as "sessionID:refreshToken".
func parseRefreshCookie(cookieValue string) (sessionID int64, refreshToken string, ok bool) {
	parts := strings.SplitN(cookieValue, ":", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id == 0 {
		return 0, "", false
	}
	return id, parts[1], true
}

// ---------------------------------------------------------------------------
// Route registration
// ---------------------------------------------------------------------------

func RegisterAuthRoutes(r gin.IRouter, svc *service.AuthService, jwtSecret string) {
	// Public auth endpoints.
	r.POST("/api/v1/auth/verifications", sendVerificationHandler(svc))
	r.POST("/api/v1/auth/verifications/confirm", confirmVerificationHandler(svc))
	r.POST("/api/v1/auth/register", registerHandler(svc))
	r.POST("/api/v1/auth/login", loginHandler(svc, jwtSecret))
	r.POST("/api/v1/auth/token/refresh", refreshTokenHandler(svc))
	r.POST("/api/v1/auth/logout", logoutHandler(svc))
	r.POST("/api/v1/auth/password/reset", resetPasswordHandler(svc))

	// /me requires authentication — apply auth middleware per-route.
	r.GET("/api/v1/auth/me", platformhttp.AuthMiddleware(jwtSecret, false), meHandler(svc))
}
