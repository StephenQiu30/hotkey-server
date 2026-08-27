package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterCollectionRoutes mounts collection control separately from
// SourceConnection CRUD. Manual collection is contributor-scoped; run retry
// and source health remain administrator-only.
func RegisterCollectionRoutes(router *gin.Engine, service collectionControlService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewCollectionHandler(service)
	api := router.Group("/api/v1", httptransport.RequireAuthentication(authenticator))
	api.GET("/monitors/:id/scans", httptransport.Wrap(handler.Scans))
	contributors := api.Group("", httptransport.RequireRoles(httptransport.RoleAnalyst, httptransport.RoleEditor, httptransport.RoleAdmin))
	contributors.POST("/monitors/:id/collect", httptransport.Wrap(handler.Manual))
	editor := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	editor.GET("/collection-runs", httptransport.Wrap(handler.List))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/collection-runs/:id/retry", httptransport.Wrap(handler.Retry))
	admin.POST("/source-connections/:id/health", httptransport.Wrap(handler.Health))
}

func RegisterAgentCollectionRoutes(router *gin.Engine, service collectionControlService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewCollectionHandler(service)
	api := router.Group("/api/v1/agent", httptransport.RequireAuthentication(authenticator), httptransport.RequireAgentScope("search.run", httptransport.RoleAnalyst, httptransport.RoleEditor, httptransport.RoleAdmin))
	api.POST("/monitors/:id/collect", httptransport.Wrap(handler.Manual))
}
