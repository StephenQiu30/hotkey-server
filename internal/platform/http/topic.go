package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// RegisterTopicRoutes registers the topic endpoints.
func RegisterTopicRoutes(r *gin.Engine, svc topic.TopicQueryService) {
	r.GET("/api/v1/monitors/:id/topics", func(c *gin.Context) {
		if _, ok := userIDFromCtx(c.Request.Context()); !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid monitor id")
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
	})
}
