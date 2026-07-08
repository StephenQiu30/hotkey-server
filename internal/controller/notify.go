package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func RegisterNotifyRoutes(r *gin.Engine, svc *service.NotifyService) {
	r.GET("/api/v1/notifications", listNotificationsHandler(svc))
	r.POST("/api/v1/notifications/:id/read", markNotificationReadHandler(svc))
}

func toNotificationResponse(n dto.Notification) vo.NotificationData {
	r := vo.NotificationData{
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

// listNotificationsHandler godoc
// @Summary List unread notifications
// @ID list-notifications
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} NotificationListResponse
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/notifications [get]
func listNotificationsHandler(svc *service.NotifyService) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		result := make([]vo.NotificationData, len(items))
		for i, n := range items {
			result[i] = toNotificationResponse(n)
		}

		RespondOK(c, result)
	}
}

// markNotificationReadHandler godoc
// @Summary Mark notification as read
// @ID mark-notification-read
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param id path int true "Notification ID"
// @Success 200 {object} MarkNotificationReadResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/notifications/{id}/read [post]
func markNotificationReadHandler(svc *service.NotifyService) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			if err == service.NotifyErrNotFound || err == service.NotifyErrNotOwned {
				respondError(c, http.StatusNotFound, err.Error())
				return
			}
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}

		RespondOK(c, vo.MarkNotificationReadData{Read: true})
	}
}
