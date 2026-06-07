package xauth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	servicexauth "github.com/StephenQiu30/hotkey-server/internal/service/xauth"
	"github.com/gin-gonic/gin"
)

// Handler manages X OAuth HTTP endpoints.
type Handler struct {
	service *servicexauth.Service
}

// New creates a new X auth handler.
func New(service *servicexauth.Service) *Handler {
	return &Handler{service: service}
}

// AuthURL returns an X OAuth authorization URL with PKCE parameters.
func (h *Handler) AuthURL(c *gin.Context) {
	state := generateState()
	result, err := h.service.GenerateAuthURL(c.Request.Context(), state)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "failed to generate auth url")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"authURL":      result.URL,
		"state":        result.State,
		"codeVerifier": result.CodeVerifier,
	})
}

// Callback handles the OAuth callback from X.
func (h *Handler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "missing code or state parameter")
		return
	}

	token, err := h.service.ExchangeCode(c.Request.Context(), servicexauth.ExchangeInput{
		Code:  code,
		State: state,
	})
	if err != nil {
		if errors.Is(err, servicexauth.ErrInvalidState) {
			writeError(c, http.StatusBadRequest, "invalid_state", "invalid or expired oauth state")
			return
		}
		writeError(c, http.StatusBadGateway, "token_exchange_failed", "failed to exchange authorization code")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken":  token.AccessToken,
		"refreshToken": token.RefreshToken,
		"expiresAt":    token.ExpiresAt,
	})
}

// Status returns the authorization status for a source.
func (h *Handler) Status(c *gin.Context) {
	sourceID := c.Query("sourceId")
	if sourceID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "missing sourceId parameter")
		return
	}

	cred, err := h.service.GetCredential(c.Request.Context(), sourceID)
	if err != nil {
		if errors.Is(err, servicexauth.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{"authorized": false})
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authorized": true,
		"expiresAt":  cred.ExpiresAt,
		"updatedAt":  cred.UpdatedAt,
	})
}

// Revoke removes stored OAuth credentials for a source.
func (h *Handler) Revoke(c *gin.Context) {
	var req struct {
		SourceID string `json:"sourceId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if req.SourceID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "sourceId is required")
		return
	}

	err := h.service.RevokeCredential(c.Request.Context(), req.SourceID)
	if err != nil {
		if errors.Is(err, servicexauth.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "credential not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func generateState() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return hex.EncodeToString([]byte("fallback-state"))
	}
	return hex.EncodeToString(data)
}
