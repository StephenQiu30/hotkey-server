package http

import (
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts every SourceConnection control-plane route at its
// stable API v1 path. Public reads still require a bearer-authenticated team
// member; writes are restricted to administrators.
func RegisterRoutes(router *gin.Engine, service *sourceapplication.Service, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/source-connections", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("", httptransport.Wrap(handler.Create))
	admin.PATCH("/:id", httptransport.Wrap(handler.Update))
	admin.POST("/:id/enable", httptransport.Wrap(handler.Enable))
	admin.POST("/:id/disable", httptransport.Wrap(handler.Disable))
	admin.POST("/:id/archive", httptransport.Wrap(handler.Archive))
	admin.POST("/:id/restore", httptransport.Wrap(handler.Restore))
}
