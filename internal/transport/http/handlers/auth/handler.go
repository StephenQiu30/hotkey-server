package auth

import (
	"errors"
	"net/http"
	"strings"

	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *serviceauth.Service
}

func New(service *serviceauth.Service) *Handler {
	return &Handler{service: service}
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *Handler) Register(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	account, err := h.service.Register(c.Request.Context(), serviceauth.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, serviceauth.ErrEmailAlreadyExists) {
			writeError(c, http.StatusConflict, "email_already_exists", "email already exists")
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": userResponse(account)})
}

func (h *Handler) Login(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	session, err := h.service.Login(c.Request.Context(), serviceauth.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
		return
	}
	writeSession(c, session)
}

func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	session, err := h.service.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_refresh_token", "invalid refresh token")
		return
	}
	writeSession(c, session)
}

func (h *Handler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if err := h.service.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_refresh_token", "invalid refresh token")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Me(c *gin.Context) {
	account, ok := CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": userResponse(account)})
}

func (h *Handler) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		account, err := h.service.CurrentUser(c.Request.Context(), token)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
			c.Abort()
			return
		}
		SetCurrentUser(c, account)
		c.Next()
	}
}

func (h *Handler) AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		account, err := h.service.RequireAdmin(c.Request.Context(), token)
		if err != nil {
			status := http.StatusForbidden
			code := "forbidden"
			if errors.Is(err, serviceauth.ErrInvalidAccessToken) {
				status = http.StatusUnauthorized
				code = "unauthorized"
			}
			writeError(c, status, code, code)
			c.Abort()
			return
		}
		SetCurrentUser(c, account)
		c.Next()
	}
}

func (h *Handler) AdminHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func writeSession(c *gin.Context, session serviceauth.Session) {
	c.JSON(http.StatusOK, gin.H{
		"accessToken":  session.AccessToken,
		"refreshToken": session.RefreshToken,
		"expiresAt":    session.ExpiresAt,
		"user":         userResponse(session.User),
	})
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
