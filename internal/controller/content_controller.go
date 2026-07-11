package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
)



func RegisterContentRoutes(r gin.IRouter, svc content.PostQueryService, mgr MonitorGetter) {
	r.GET("/api/v1/monitors/:id/posts", listMonitorPostsHandler(svc, mgr))
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
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 403 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/monitors/{id}/posts [get]
func listMonitorPostsHandler(svc content.PostQueryService, mgr MonitorGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "unauthorized")
			return
		}

		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid monitor id")
			return
		}

		m, err := mgr.GetByID(ctx, id)
		if err != nil {
			switch {
			case err == service.MonitorErrNotFound:
				platformhttp.RespondError(c, enum.ErrorCodeNotFound, "monitor not found")
			default:
				platformhttp.RespondInternalError(c)
			}
			return
		}
		if m.UserID != userID {
			platformhttp.RespondError(c, enum.ErrorCodeForbidden, "not authorized")
			return
		}

		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

		posts, err := svc.ListPostsByMonitor(id, limit, offset)
		if err != nil {
			platformhttp.RespondInternalError(c)
			return
		}
		if posts == nil {
			posts = []content.PostSummary{}
		}

		platformhttp.RespondOK(c, posts)
	}
}
