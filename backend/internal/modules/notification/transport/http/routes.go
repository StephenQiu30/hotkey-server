package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, handler *Handler, authenticator httptransport.Authenticator) {
	if router == nil || handler == nil {
		return
	}
	api := router.Group("/api/v1/notifications", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/stream", handler.Stream)
}
