package xauth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/httputil"
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

// AuthURL returns an X OAuth authorization URL. The PKCE code verifier is stored server-side only.
func (h *Handler) AuthURL(c *gin.Context) {
	state := generateState()
	result, err := h.service.GenerateAuthURL(c.Request.Context(), state)
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "failed to generate auth url")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"authURL": result.URL,
		"state":   result.State,
	})
}

// Callback handles the OAuth callback from X and stores the credential.
func (h *Handler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	sourceID := c.Query("sourceId")
	if code == "" || state == "" || sourceID == "" {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "missing code, state, or sourceId parameter")
		return
	}

	token, err := h.service.ExchangeCode(c.Request.Context(), servicexauth.ExchangeInput{
		Code:     code,
		State:    state,
		SourceID: sourceID,
	})
	if err != nil {
		if errors.Is(err, servicexauth.ErrInvalidState) {
			httputil.WriteError(c, http.StatusBadRequest, "invalid_state", "invalid or expired oauth state")
			return
		}
		httputil.WriteError(c, http.StatusBadGateway, "token_exchange_failed", "failed to exchange authorization code")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken": token.AccessToken,
		"expiresAt":   token.ExpiresAt,
		"sourceId":    sourceID,
	})
}

// Status returns the authorization status for a source.
func (h *Handler) Status(c *gin.Context) {
	sourceID := c.Query("sourceId")
	if sourceID == "" {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "missing sourceId parameter")
		return
	}

	cred, err := h.service.GetCredential(c.Request.Context(), sourceID)
	if err != nil {
		if errors.Is(err, servicexauth.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{"authorized": false})
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
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
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if req.SourceID == "" {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "sourceId is required")
		return
	}

	err := h.service.RevokeCredential(c.Request.Context(), req.SourceID)
	if err != nil {
		if errors.Is(err, servicexauth.ErrNotFound) {
			httputil.WriteError(c, http.StatusNotFound, "not_found", "credential not found")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

func generateState() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		panic(fmt.Sprintf("generate state: %v", err))
	}
	return hex.EncodeToString(data)
}
