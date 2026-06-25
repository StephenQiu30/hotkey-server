package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// RegisterContentRoutes registers the content (posts) endpoints.
func RegisterContentRoutes(r *gin.Engine, svc content.PostQueryService) {
	r.GET("/api/v1/monitors/:id/posts", func(c *gin.Context) {
		if _, ok := userIDFromCtx(c.Request.Context()); !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid monitor id")
			return
		}

		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

		posts, err := svc.ListPostsByMonitor(id, limit, offset)
		if err != nil {
			respondInternalError(c)
			return
		}
		if posts == nil {
			posts = []content.PostSummary{}
		}

		c.JSON(http.StatusOK, posts)
	})
}
