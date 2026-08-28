package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service searchService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/search",
		httptransport.RequireAuthentication(authenticator),
		httptransport.RequireRoles(httptransport.RoleViewer, httptransport.RoleAnalyst, httptransport.RoleEditor, httptransport.RoleAdmin),
	)
	api.GET("", httptransport.Wrap(handler.List))
}
