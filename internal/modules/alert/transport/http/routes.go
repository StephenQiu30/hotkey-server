package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service alertService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/alerts", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.POST("/:id/acknowledge", httptransport.Wrap(handler.Acknowledge))
	api.POST("/:id/resolve", httptransport.Wrap(handler.Resolve))
	elevated := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	elevated.POST("/:id/suppress", httptransport.Wrap(handler.Suppress))
}
