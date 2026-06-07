package mail

import (
	"net/http"

	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

type EmailPreferenceService interface {
	GetEmailPreference(userID string) (EmailPreference, error)
	SetEmailPreference(userID string, pref EmailPreference) error
}

type EmailPreference struct {
	EmailEnabled  bool
	DailyEnabled  bool
	WeeklyEnabled bool
}

type Handler struct {
	service EmailPreferenceService
}

func New(service EmailPreferenceService) *Handler {
	return &Handler{service: service}
}

type emailPreferenceRequest struct {
	EmailEnabled  *bool `json:"emailEnabled"`
	DailyEnabled  *bool `json:"dailyEnabled"`
	WeeklyEnabled *bool `json:"weeklyEnabled"`
}

func (h *Handler) GetEmailPreference(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
		return
	}
	pref, err := h.service.GetEmailPreference(account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "internal error"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"emailEnabled":  pref.EmailEnabled,
		"dailyEnabled":  pref.DailyEnabled,
		"weeklyEnabled": pref.WeeklyEnabled,
	})
}

func (h *Handler) SetEmailPreference(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
		return
	}
	var req emailPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "invalid request"}})
		return
	}
	// Get current preference, then apply partial updates
	current, err := h.service.GetEmailPreference(account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "internal error"}})
		return
	}
	if req.EmailEnabled != nil {
		current.EmailEnabled = *req.EmailEnabled
	}
	if req.DailyEnabled != nil {
		current.DailyEnabled = *req.DailyEnabled
	}
	if req.WeeklyEnabled != nil {
		current.WeeklyEnabled = *req.WeeklyEnabled
	}
	if err := h.service.SetEmailPreference(account.ID, current); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "internal error"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"emailEnabled":  current.EmailEnabled,
		"dailyEnabled":  current.DailyEnabled,
		"weeklyEnabled": current.WeeklyEnabled,
	})
}
