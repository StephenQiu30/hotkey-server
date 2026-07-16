// Package http adapts the identity application service to the public HTTP
// contract. It contains no concrete adapter imports and all JSON is emitted
// through the platform Result helpers.
package http

import (
	"context"
	stdhttp "net/http"
	"strconv"

	identityapplication "github.com/StephenQiu30/hotkey-server/internal/modules/identity/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "hotkey_refresh"
	refreshCookiePath = "/api/v1/auth"
)

// identityService is the application boundary needed by this transport. The
// production application Service satisfies it and tests use safe fakes.
type identityService interface {
	RequestVerification(context.Context, domain.VerificationPurpose, string) error
	ConfirmVerification(context.Context, domain.VerificationPurpose, string, string) (domain.VerificationTicket, error)
	Register(context.Context, identityapplication.RegisterInput) (*domain.User, error)
	Login(context.Context, identityapplication.Credentials) (identityapplication.Authentication, error)
	Refresh(context.Context, string) (identityapplication.Authentication, error)
	Logout(context.Context, *domain.Subject, string) error
	CurrentUser(context.Context, domain.Subject) (*domain.User, error)
	ChangePassword(context.Context, domain.Subject, string, string) error
	ConfirmPasswordReset(context.Context, string, string) error
	ListUsers(context.Context) ([]domain.User, error)
	UpdateUser(context.Context, domain.Subject, int64, identityapplication.UserUpdate) (*domain.User, error)
	DeleteUser(context.Context, domain.Subject, int64) (*domain.User, error)
	RestoreUser(context.Context, domain.Subject, int64) (*domain.User, error)
}

type Handler struct {
	service      identityService
	cookieSecure bool
}

func NewHandler(service identityService, cfg config.Config) *Handler {
	return &Handler{service: service, cookieSecure: cfg.Authentication.RefreshCookieSecure}
}

// RequestVerification accepts either registration or password-reset delivery
// without consulting account existence.
// @Summary Request an email verification code
// @Tags identity
// @Accept json
// @Produce json
// @Param request body RequestVerificationRequest true "verification request"
// @Success 200 {object} IdentityResult[EmptyResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 429 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/email-verifications [post]
func (handler *Handler) RequestVerification(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request RequestVerificationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	if err := handler.service.RequestVerification(c.Request.Context(), domain.VerificationPurpose(request.Purpose), request.Email); err != nil {
		return err
	}
	httptransport.Empty(c)
	return nil
}

// ConfirmVerification exchanges a code for one short-lived, single-use ticket.
// @Summary Confirm an email verification code
// @Tags identity
// @Accept json
// @Produce json
// @Param request body ConfirmVerificationRequest true "verification confirmation"
// @Success 200 {object} IdentityResult[ConfirmVerificationResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 429 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/email-verifications/confirm [post]
func (handler *Handler) ConfirmVerification(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request ConfirmVerificationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	ticket, err := handler.service.ConfirmVerification(c.Request.Context(), domain.VerificationPurpose(request.Purpose), request.Email, request.Code)
	if err != nil {
		return err
	}
	httptransport.OK(c, ConfirmVerificationResponse{VerificationTicket: ticket.Token})
	return nil
}

// Register creates an active viewer after consuming a registration ticket.
// @Summary Register an email-verified viewer
// @Tags identity
// @Accept json
// @Produce json
// @Param request body RegistrationRequest true "registration request"
// @Success 201 {object} IdentityResult[UserResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 409 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/registrations [post]
func (handler *Handler) Register(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request RegistrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	user, err := handler.service.Register(c.Request.Context(), identityapplication.RegisterInput{VerificationTicket: request.VerificationTicket, Password: request.Password, DisplayName: request.DisplayName})
	if err != nil {
		return err
	}
	httptransport.Created(c, userResponse(*user))
	return nil
}

// Login creates a refresh session and returns an in-memory access token.
// @Summary Log in with email and password
// @Tags identity
// @Accept json
// @Produce json
// @Param request body LoginRequest true "credentials"
// @Success 200 {object} IdentityResult[AuthenticationResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/login [post]
func (handler *Handler) Login(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	authentication, err := handler.service.Login(c.Request.Context(), identityapplication.Credentials{Email: request.Email, Password: request.Password})
	if err != nil {
		return err
	}
	handler.setRefreshCookie(c, authentication.RefreshToken)
	httptransport.OK(c, authenticationResponse(authentication))
	return nil
}

// Refresh rotates the opaque refresh cookie after the cookie Origin guard.
// @Summary Rotate the refresh credential
// @Tags identity
// @Produce json
// @Success 200 {object} IdentityResult[AuthenticationResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 403 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/refresh [post]
func (handler *Handler) Refresh(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	refreshToken, _ := c.Cookie(refreshCookieName)
	authentication, err := handler.service.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		return err
	}
	handler.setRefreshCookie(c, authentication.RefreshToken)
	httptransport.OK(c, authenticationResponse(authentication))
	return nil
}

// Logout is a success even when credentials are stale, and always clears the
// browser refresh cookie once the explicit Origin check has passed.
// @Summary Revoke the current session and clear the refresh cookie
// @Tags identity
// @Produce json
// @Success 200 {object} IdentityResult[EmptyResponse]
// @Failure 403 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/logout [post]
func (handler *Handler) Logout(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	refreshToken, _ := c.Cookie(refreshCookieName)
	handler.clearRefreshCookie(c)
	if err := handler.service.Logout(c.Request.Context(), optionalSubject(c), refreshToken); err != nil {
		return err
	}
	httptransport.Empty(c)
	return nil
}

// Me returns the database-backed active subject rather than bearer claims.
// @Summary Get the current active user
// @Tags identity
// @Produce json
// @Security BearerAuth
// @Success 200 {object} IdentityResult[UserResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/me [get]
func (handler *Handler) Me(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	subject, ok := protectedSubject(c)
	if !ok {
		return unauthenticated()
	}
	user, err := handler.service.CurrentUser(c.Request.Context(), subject)
	if err != nil {
		return err
	}
	httptransport.OK(c, userResponse(*user))
	return nil
}

// ChangePassword revokes all sessions and clears this browser's refresh cookie.
// @Summary Change the current user password
// @Tags identity
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ChangePasswordRequest true "password change"
// @Success 200 {object} IdentityResult[EmptyResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/password [post]
func (handler *Handler) ChangePassword(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request ChangePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	subject, ok := protectedSubject(c)
	if !ok {
		return unauthenticated()
	}
	if err := handler.service.ChangePassword(c.Request.Context(), subject, request.CurrentPassword, request.NewPassword); err != nil {
		return err
	}
	handler.clearRefreshCookie(c)
	httptransport.Empty(c)
	return nil
}

// ConfirmPasswordReset consumes a reset ticket and revokes prior sessions.
// @Summary Confirm a password reset
// @Tags identity
// @Accept json
// @Produce json
// @Param request body ConfirmPasswordResetRequest true "password reset"
// @Success 200 {object} IdentityResult[EmptyResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/auth/password-resets/confirm [post]
func (handler *Handler) ConfirmPasswordReset(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request ConfirmPasswordResetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	if err := handler.service.ConfirmPasswordReset(c.Request.Context(), request.VerificationTicket, request.Password); err != nil {
		return err
	}
	handler.clearRefreshCookie(c)
	httptransport.Empty(c)
	return nil
}

// ListUsers lists safe user DTOs for administrators.
// @Summary List users
// @Tags identity
// @Produce json
// @Security BearerAuth
// @Success 200 {object} IdentityResult[[]UserResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 403 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/users [get]
func (handler *Handler) ListUsers(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	users, err := handler.service.ListUsers(c.Request.Context())
	if err != nil {
		return err
	}
	httptransport.OK(c, userResponses(users))
	return nil
}

// UpdateUser changes only one user's role and/or active status.
// @Summary Change a user role or status
// @Tags identity
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "user ID"
// @Param request body UpdateUserRequest true "role/status update"
// @Success 200 {object} IdentityResult[UserResponse]
// @Failure 400 {object} IdentityResult[EmptyResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 403 {object} IdentityResult[EmptyResponse]
// @Failure 409 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/users/{id} [patch]
func (handler *Handler) UpdateUser(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	var request UpdateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidRequest(err)
	}
	userID, err := pathUserID(c)
	if err != nil {
		return err
	}
	subject, ok := protectedSubject(c)
	if !ok {
		return unauthenticated()
	}
	update := identityapplication.UserUpdate{}
	if request.Role != nil {
		value := domain.Role(*request.Role)
		update.Role = &value
	}
	if request.Status != nil {
		value := domain.UserStatus(*request.Status)
		update.Status = &value
	}
	user, err := handler.service.UpdateUser(c.Request.Context(), subject, userID, update)
	if err != nil {
		return err
	}
	httptransport.OK(c, userResponse(*user))
	return nil
}

// DeleteUser soft-deletes a user and revokes all sessions.
// @Summary Soft-delete a user
// @Tags identity
// @Produce json
// @Security BearerAuth
// @Param id path int true "user ID"
// @Success 200 {object} IdentityResult[UserResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 403 {object} IdentityResult[EmptyResponse]
// @Failure 409 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/users/{id} [delete]
func (handler *Handler) DeleteUser(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	userID, err := pathUserID(c)
	if err != nil {
		return err
	}
	subject, ok := protectedSubject(c)
	if !ok {
		return unauthenticated()
	}
	user, err := handler.service.DeleteUser(c.Request.Context(), subject, userID)
	if err != nil {
		return err
	}
	httptransport.OK(c, userResponse(*user))
	return nil
}

// RestoreUser restores a soft-deleted user as disabled without sessions.
// @Summary Restore a deleted user as disabled
// @Tags identity
// @Produce json
// @Security BearerAuth
// @Param id path int true "user ID"
// @Success 200 {object} IdentityResult[UserResponse]
// @Failure 401 {object} IdentityResult[EmptyResponse]
// @Failure 403 {object} IdentityResult[EmptyResponse]
// @Failure 409 {object} IdentityResult[EmptyResponse]
// @Failure 503 {object} IdentityResult[EmptyResponse]
// @Router /api/v1/users/{id}/restore [post]
func (handler *Handler) RestoreUser(c *gin.Context) error {
	httptransport.SetModule(c, "identity")
	userID, err := pathUserID(c)
	if err != nil {
		return err
	}
	subject, ok := protectedSubject(c)
	if !ok {
		return unauthenticated()
	}
	user, err := handler.service.RestoreUser(c.Request.Context(), subject, userID)
	if err != nil {
		return err
	}
	httptransport.OK(c, userResponse(*user))
	return nil
}

func (handler *Handler) setRefreshCookie(c *gin.Context, value string) {
	c.SetSameSite(stdhttp.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, value, 0, refreshCookiePath, "", handler.cookieSecure, true)
}

func (handler *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(stdhttp.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", handler.cookieSecure, true)
}

func authenticationResponse(authentication identityapplication.Authentication) AuthenticationResponse {
	return AuthenticationResponse{AccessToken: authentication.AccessToken, User: userResponse(authentication.User)}
}

func pathUserID(c *gin.Context) (int64, error) {
	value, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || value <= 0 {
		return 0, invalidRequest(err)
	}
	return value, nil
}

func invalidRequest(cause error) error {
	return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", cause)
}

func unauthenticated() error {
	return sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
}
