package authorization

import (
	"errors"
	"net/http"

	"github.com/StephenQiu30/hotkey-server/internal/domain/authorization"
	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	azService *serviceauth.AuthorizationService
}

func New(azService *serviceauth.AuthorizationService) *Handler {
	return &Handler{azService: azService}
}

type connectRequest struct {
	Platform       string  `json:"platform"`
	PlatformUserID string  `json:"platformUserId"`
	DisplayName    string  `json:"displayName"`
	AccessToken    string  `json:"accessToken"`
	RefreshToken   string  `json:"refreshToken"`
}

func (h *Handler) Connect(c *gin.Context) {
	account, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req connectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
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
			writeError(c, http.StatusBadRequest, "invalid_platform", "invalid platform")
			return
		}
		if errors.Is(err, authorization.ErrUniqueViolation) {
			writeError(c, http.StatusConflict, "already_connected", "platform already connected")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"authorization": authorizationResponse(az)})
}

func (h *Handler) List(c *gin.Context) {
	account, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	authorizations, err := h.azService.ListByUser(c.Request.Context(), account.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	result := make([]gin.H, len(authorizations))
	for i, az := range authorizations {
		result[i] = authorizationResponse(az)
	}
	c.JSON(http.StatusOK, gin.H{"authorizations": result})
}

func (h *Handler) Test(c *gin.Context) {
	account, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	azID := c.Param("id")
	az, err := h.azService.HealthCheck(c.Request.Context(), account.ID, azID)
	if err != nil {
		if errors.Is(err, authorization.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "authorization not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"authorization": authorizationResponse(az)})
}

func (h *Handler) Disconnect(c *gin.Context) {
	account, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	azID := c.Param("id")
	err := h.azService.Disconnect(c.Request.Context(), account.ID, azID)
	if err != nil {
		if errors.Is(err, authorization.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "authorization not found")
			return
		}
		if errors.Is(err, authorization.ErrAlreadyRevoked) {
			writeError(c, http.StatusConflict, "already_revoked", "authorization already revoked")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteAccount(c *gin.Context) {
	account, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := h.azService.DeleteAccount(c.Request.Context(), account.ID); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
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

func currentUser(c *gin.Context) (user.User, bool) {
	val, exists := c.Get("currentUser")
	if !exists {
		return user.User{}, false
	}
	account, ok := val.(user.User)
	return account, ok
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
