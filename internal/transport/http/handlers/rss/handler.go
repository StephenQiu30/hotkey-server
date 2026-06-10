package rss

import (
	"errors"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/httputil"
	"net/http"
	"strings"

	servicerss "github.com/StephenQiu30/hotkey-server/internal/service/rss"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *servicerss.Service
}

func New(service *servicerss.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PublicChannel(c *gin.Context) {
	doc, err := h.service.PublicChannelFeed(c.Request.Context(), strings.TrimSuffix(c.Param("channelCode"), ".xml"))
	if err != nil {
		if errors.Is(err, servicerss.ErrInvalidInput) {
			httputil.WriteError(c, http.StatusBadRequest, "invalid_request", "invalid request")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeRSS(c, doc)
}

func (h *Handler) PrivateUser(c *gin.Context) {
	doc, err := h.service.PrivateUserFeed(c.Request.Context(), c.Param("token"))
	if err != nil {
		if errors.Is(err, servicerss.ErrFeedNotFound) || errors.Is(err, servicerss.ErrFeedDisabled) {
			httputil.WriteError(c, http.StatusNotFound, "rss_feed_not_found", "rss feed not found")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeRSS(c, doc)
}

func (h *Handler) GetUserFeed(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	feed, err := h.service.UserFeed(c.Request.Context(), account.ID)
	if err != nil {
		if errors.Is(err, servicerss.ErrFeedNotFound) {
			c.JSON(http.StatusOK, gin.H{"userId": account.ID, "enabled": false})
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.JSON(http.StatusOK, feedResponse(feed, ""))
}

func (h *Handler) ResetUserFeed(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	feed, token, err := h.service.ResetUserFeed(c.Request.Context(), account.ID)
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.JSON(http.StatusOK, feedResponse(feed, token))
}

func (h *Handler) DisableUserFeed(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		httputil.WriteError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if err := h.service.DisableUserFeed(c.Request.Context(), account.ID); err != nil && !errors.Is(err, servicerss.ErrFeedNotFound) {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.Status(http.StatusNoContent)
}

func writeRSS(c *gin.Context, doc servicerss.Document) {
	body, err := doc.XML()
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.Data(http.StatusOK, "application/rss+xml; charset=utf-8", body)
}

func feedResponse(feed servicerss.Feed, token string) gin.H {
	body := gin.H{
		"userId":         feed.UserID,
		"enabled":        feed.Enabled,
		"lastAccessedAt": feed.LastAccessedAt,
		"createdAt":      feed.CreatedAt,
		"updatedAt":      feed.UpdatedAt,
	}
	if token != "" {
		body["token"] = token
	}
	return body
}
