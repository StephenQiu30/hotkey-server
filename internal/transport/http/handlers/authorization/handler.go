package authorization

import (
	"errors"
	"net/http"

	"github.com/StephenQiu30/hotkey-server/internal/domain/authorization"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/httputil"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	azService *serviceauth.AuthorizationService
}

func New(azService *serviceauth.AuthorizationService) *Handler {
	return &Handler{azService: azService}
}

type connectRequest struct {
	Platform       string `json:"platform" binding:"required"`
	PlatformUserID string `json:"platformUserId"`
	DisplayName    string `json:"displayName"`
	AccessToken    string `json:"accessToken" binding:"required"`
	RefreshToken   string `json:"refreshToken"`
}

func (h *Handler) Connect(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req connectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}

	az, err := h.azService.Connect(c.Request.Context(), serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.Platform(req.Platform),
		PlatformUserID: req.PlatformUserID,
		DisplayName:    req.DisplayName,
		AccessToken:    req.AccessToken,
		RefreshToken:   req.RefreshToken,
	})
	if err != nil {
		if errors.Is(err, authorization.ErrInvalidPlatform) {
			httputil.WriteError(c, http.StatusBadRequest, "invalid_platform", "invalid platform")
			return
		}
		if errors.Is(err, authorization.ErrUniqueViolation) {
			httputil.WriteError(c, http.StatusConflict, "already_connected", "platform already connected")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"authorization": authorizationResponse(az)})
}

func (h *Handler) List(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	authorizations, err := h.azService.ListByUser(c.Request.Context(), account.ID)
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	result := make([]gin.H, len(authorizations))
	for i, az := range authorizations {
		result[i] = authorizationResponse(az)
	}
	c.JSON(http.StatusOK, gin.H{"authorizations": result})
}

func (h *Handler) Test(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	azID := c.Param("id")
	az, err := h.azService.HealthCheck(c.Request.Context(), account.ID, azID)
	if err != nil {
		if errors.Is(err, authorization.ErrNotFound) {
			httputil.WriteError(c, http.StatusNotFound, "not_found", "authorization not found")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"authorization": authorizationResponse(az)})
}

func (h *Handler) Disconnect(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	azID := c.Param("id")
	err := h.azService.Disconnect(c.Request.Context(), account.ID, azID)
	if err != nil {
		if errors.Is(err, authorization.ErrNotFound) {
			httputil.WriteError(c, http.StatusNotFound, "not_found", "authorization not found")
			return
		}
		if errors.Is(err, authorization.ErrAlreadyRevoked) {
			httputil.WriteError(c, http.StatusConflict, "already_revoked", "authorization already revoked")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteAccount(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := h.azService.DeleteAccount(c.Request.Context(), account.ID); err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.Status(http.StatusNoContent)
}

func authorizationResponse(az authorization.Authorization) gin.H {
	return gin.H{
		"id":             az.ID,
		"platform":       az.Platform,
		"platformUserId": az.PlatformUserID,
		"displayName":    az.DisplayName,
		"status":         az.Status,
		"connectedAt":    az.ConnectedAt,
		"lastCheckedAt":  az.LastCheckedAt,
		"expiresAt":      az.ExpiresAt,
	}
}
