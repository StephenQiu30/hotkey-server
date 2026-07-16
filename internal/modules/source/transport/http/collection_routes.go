package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterCollectionRoutes mounts the administrator-only collection control
// surface separately from SourceConnection CRUD, while retaining the same
// authentication and Result error semantics.
func RegisterCollectionRoutes(router *gin.Engine, service collectionControlService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewCollectionHandler(service)
	api := router.Group("/api/v1", httptransport.RequireAuthentication(authenticator))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("/collection-runs", httptransport.Wrap(handler.List))
	admin.POST("/collection-runs/:id/retry", httptransport.Wrap(handler.Retry))
	admin.POST("/source-connections/:id/health", httptransport.Wrap(handler.Health))
}
