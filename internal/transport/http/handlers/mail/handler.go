package mail

import (
	"errors"
	"net/http"

	"github.com/StephenQiu30/hotkey-server/internal/service/channel"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	channelService *channel.Service
}

func New(channelService *channel.Service) *Handler {
	return &Handler{channelService: channelService}
}

type EmailPreferences struct {
	DailySendAt   string `json:"dailySendAt"`
	WeeklyEnabled *bool  `json:"weeklyEnabled,omitempty"`
	WeeklySendAt  string `json:"weeklySendAt"`
}

func (h *Handler) GetEmailPreferences(c *gin.Context) {
	account, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	dailySendAt, err := h.channelService.UserDailySendAt(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get daily send at"})
		return
	}

	weeklyEnabled, err := h.channelService.UserWeeklyEnabled(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get weekly enabled"})
		return
	}

	weeklySendAt, err := h.channelService.UserWeeklySendAt(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get weekly send at"})
		return
	}

	c.JSON(http.StatusOK, EmailPreferences{
		DailySendAt:   dailySendAt,
		WeeklyEnabled: &weeklyEnabled,
		WeeklySendAt:  weeklySendAt,
	})
}

func (h *Handler) UpdateEmailPreferences(c *gin.Context) {
	account, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req EmailPreferences
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.DailySendAt != "" {
		if err := h.channelService.SetUserDailySendAt(c.Request.Context(), channel.UserDailySendAtInput{
			UserID:      account.ID,
			DailySendAt: req.DailySendAt,
		}); err != nil {
			if errors.Is(err, channel.ErrInvalidInput) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid daily send time"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set daily send time"})
			}
			return
		}
	}

	if req.WeeklyEnabled != nil {
		if err := h.channelService.SetUserWeeklyEnabled(c.Request.Context(), account.ID, *req.WeeklyEnabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set weekly enabled"})
			return
		}
	}

	if req.WeeklySendAt != "" {
		if err := h.channelService.SetUserWeeklySendAt(c.Request.Context(), account.ID, req.WeeklySendAt); err != nil {
			if errors.Is(err, channel.ErrInvalidInput) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid weekly send time"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set weekly send time"})
			}
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
