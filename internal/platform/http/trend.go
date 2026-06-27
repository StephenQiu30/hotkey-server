package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// RegisterTrendRoutes registers the trend endpoints.
func RegisterTrendRoutes(r *gin.Engine, svc trend.TrendQueryService) {
	r.GET("/api/v1/monitors/:id/trends", func(c *gin.Context) {
		if _, ok := userIDFromCtx(c.Request.Context()); !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid monitor id")
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
	})

	r.GET("/api/v1/topics/:id/trends", func(c *gin.Context) {
		if _, ok := userIDFromCtx(c.Request.Context()); !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid topic id")
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
	})
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
