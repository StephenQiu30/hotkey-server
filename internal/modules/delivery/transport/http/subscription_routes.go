package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterSubscriptionRoutes(router *gin.Engine, service subscriptionService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewSubscriptionHandler(service)
	api := router.Group("/api/v1/report-subscriptions", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.POST("", httptransport.Wrap(handler.Create))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.PATCH("/:id", httptransport.Wrap(handler.Update))
	api.POST("/:id/rss-token/rotate", httptransport.Wrap(handler.RotateToken))
}
