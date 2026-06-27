package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
)

// RegisterNotifyRoutes registers the notification endpoints.
func RegisterNotifyRoutes(r *gin.Engine, svc *notify.Service) {
	r.GET("/api/v1/notifications", func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		items, err := svc.ListUnread(c.Request.Context(), userID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}

		result := make([]NotificationResponse, len(items))
		for i, n := range items {
			result[i] = toNotificationResponse(n)
		}

		RespondOK(c, result)
	})

	r.POST("/api/v1/notifications/:id/read", func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid notification id")
			return
		}

		if err := svc.MarkRead(c.Request.Context(), userID, id); err != nil {
			if err == notify.ErrNotFound || err == notify.ErrNotOwned {
				respondError(c, http.StatusNotFound, err.Error())
				return
			}
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}

		RespondOK(c, gin.H{"read": true})
	})
}

type NotificationResponse struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"user_id"`
	AlertID        int64   `json:"alert_id"`
	Channel        string  `json:"channel"`
	DeliveryStatus string  `json:"delivery_status"`
	ReadAt         *string `json:"read_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

func toNotificationResponse(n notify.Notification) NotificationResponse {
	r := NotificationResponse{
		ID: n.ID, UserID: n.UserID, AlertID: n.AlertID,
		Channel: n.Channel, DeliveryStatus: n.DeliveryStatus,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format(time.RFC3339)
		r.ReadAt = &s
	}
	return r
}
