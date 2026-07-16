package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the complete administrator-only model-profile control
// plane. Authentication and role guards run before every application call.
func RegisterRoutes(router *gin.Engine, service modelProfileService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewHandler(service)
	profiles := router.Group("/api/v1/ai/model-profiles", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	profiles.GET("", httptransport.Wrap(handler.List))
	profiles.POST("", httptransport.Wrap(handler.Create))
	profiles.GET("/:id", httptransport.Wrap(handler.Get))
	profiles.PATCH("/:id", httptransport.Wrap(handler.Update))
	profiles.DELETE("/:id", httptransport.Wrap(handler.Delete))
	profiles.POST("/:id/restore", httptransport.Wrap(handler.Restore))
}
