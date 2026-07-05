package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

func RegisterContentRoutes(r *gin.Engine, svc content.PostQueryService) {
	r.GET("/api/v1/monitors/:id/posts", listMonitorPostsHandler(svc))
}

// listMonitorPostsHandler godoc
// @Summary List posts for a monitor
// @ID list-posts
// @Tags content
// @Produce json
// @Security BearerAuth
// @Param id path int true "Monitor ID"
// @Param limit query int false "Limit" default(20)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} PostListResponse
// @Failure 400 {object} ErrorBody
// @Failure 401 {object} ErrorBody
// @Failure 500 {object} ErrorBody
// @Router /api/v1/monitors/{id}/posts [get]
func listMonitorPostsHandler(svc content.PostQueryService) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		RespondOK(c, posts)
	}
}
