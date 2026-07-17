package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service reportService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/reports", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.POST("/:id/preview", httptransport.Wrap(handler.Preview))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/:id/build", httptransport.Wrap(handler.Build))
	admin.POST("/:id/publish", httptransport.Wrap(handler.Publish))
}
