package http

import (
	application "github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewHandler(read, lifecycle, governance)
	api := router.Group("/api/v1/events", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.GET("/:id/contents", httptransport.Wrap(handler.ListMembers))
	editor := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	editor.POST("/:id/contents/:content_id/lock", httptransport.Wrap(handler.SetMemberLock))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/:id/lifecycle", httptransport.Wrap(handler.Transition))
	admin.POST("/:id/merge", httptransport.Wrap(handler.Merge))
	admin.POST("/:id/split", httptransport.Wrap(handler.Split))
}
