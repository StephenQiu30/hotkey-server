package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterPushSubscriptionRoutes(router *gin.Engine, handler *PushSubscriptionHandler, authenticator httptransport.Authenticator) {
	group := router.Group("/api/v1/notifications", httptransport.RequireAuthentication(authenticator))
	group.GET("/push-capability", httptransport.Wrap(handler.Capability))
	group.GET("/push-subscriptions", httptransport.Wrap(handler.List))
	group.POST("/push-subscriptions", httptransport.Wrap(handler.Register))
	group.PUT("/push-subscriptions/:id", httptransport.Wrap(handler.Update))
	group.DELETE("/push-subscriptions/:id", httptransport.Wrap(handler.Disable))
}
