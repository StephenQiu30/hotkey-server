package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/service"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

var _ platformhttp.ErrorBody


func RegisterTopicRoutes(r *gin.Engine, svc service.TopicQueryService, mgr MonitorGetter) {
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
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 403 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/monitors/{id}/topics [get]
func listMonitorTopicsHandler(svc service.TopicQueryService, mgr MonitorGetter) gin.HandlerFunc {
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
			case err == service.MonitorErrNotFound:
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
			topics = []service.TopicSummary{}
		}

		RespondOK(c, topics)
	}
}
