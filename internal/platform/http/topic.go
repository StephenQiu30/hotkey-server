package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

func RegisterTopicRoutes(r *gin.Engine, svc topic.TopicQueryService, mgr MonitorGetter) {
	r.GET("/api/v1/monitors/:id/topics", listMonitorTopicsHandler(svc, mgr))
}

// listMonitorTopicsHandler godoc
// @Summary List topics for a monitor
// @ID list-topics
// @Tags topics
// @Produce json
// @Security BearerAuth
// @Param id path int true "Monitor ID"
// @Success 200 {object} TopicListResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 403 {object} ErrorBody
// @Failure 404 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors/{id}/topics [get]
func listMonitorTopicsHandler(svc topic.TopicQueryService, mgr MonitorGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid monitor id")
			return
		}

		m, err := mgr.GetByID(ctx, id)
		if err != nil {
			switch {
			case err == monitor.ErrNotFound:
				respondError(c, http.StatusNotFound, "monitor not found")
			default:
				respondInternalError(c)
			}
			return
		}
		if m.UserID != userID {
			respondError(c, http.StatusForbidden, "not authorized")
			return
		}

		topics, err := svc.ListByMonitor(id)
		if err != nil {
			respondInternalError(c)
			return
		}
		if topics == nil {
			topics = []topic.TopicSummary{}
		}

		RespondOK(c, topics)
	}
}
