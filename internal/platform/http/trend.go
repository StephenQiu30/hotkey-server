package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TopicMonitorIDGetter fetches the monitor ID that owns a topic.
type TopicMonitorIDGetter interface {
	GetMonitorID(ctx context.Context, topicID int64) (int64, error)
}

func RegisterTrendRoutes(r *gin.Engine, svc trend.TrendQueryService, monitorGetter MonitorGetter, topicMonitorGetter TopicMonitorIDGetter) {
	r.GET("/api/v1/monitors/:id/trends", monitorTrendsHandler(svc, monitorGetter))
	r.GET("/api/v1/topics/:id/trends", topicTrendsHandler(svc, monitorGetter, topicMonitorGetter))
}

func parseSince(s string) time.Time {
	if s == "" {
		return time.Now().Add(-24 * time.Hour)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().Add(-24 * time.Hour)
	}
	return t
}

// monitorTrendsHandler godoc
// @Summary Get monitor trends
// @ID get-monitor-trends
// @Tags trends
// @Produce json
// @Security BearerAuth
// @Param id path int true "Monitor ID"
// @Param since query string false "RFC3339 start time"
// @Success 200 {object} TrendListResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 403 {object} ErrorBody
// @Failure 404 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors/{id}/trends [get]
func monitorTrendsHandler(svc trend.TrendQueryService, mgr MonitorGetter) gin.HandlerFunc {
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

		since := parseSince(c.Query("since"))
		points, err := svc.GetMonitorTrends(id, since)
		if err != nil {
			respondInternalError(c)
			return
		}
		if points == nil {
			points = []trend.TrendPoint{}
		}

		RespondOK(c, points)
	}
}

// topicTrendsHandler godoc
// @Summary Get topic trends
// @ID get-topic-trends
// @Tags trends
// @Produce json
// @Security BearerAuth
// @Param id path int true "Topic ID"
// @Param since query string false "RFC3339 start time"
// @Success 200 {object} TrendListResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 403 {object} ErrorBody
// @Failure 404 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/topics/{id}/trends [get]
func topicTrendsHandler(svc trend.TrendQueryService, monitorGetter MonitorGetter, topicMonitorGetter TopicMonitorIDGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid topic id")
			return
		}

		monitorID, err := topicMonitorGetter.GetMonitorID(ctx, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "topic not found")
			return
		}

		m, err := monitorGetter.GetByID(ctx, monitorID)
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

		since := parseSince(c.Query("since"))
		points, err := svc.GetTopicTrends(id, since)
		if err != nil {
			respondInternalError(c)
			return
		}
		if points == nil {
			points = []trend.TrendPoint{}
		}

		RespondOK(c, points)
	}
}
