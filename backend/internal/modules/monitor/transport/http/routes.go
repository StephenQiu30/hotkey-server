package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the fixed Monitor control-plane routes. Authorization
// is explicit at the boundary; application services repeat the rule so calls
// outside HTTP remain safe.
func RegisterRoutes(router *gin.Engine, service monitorService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/monitors", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.GET("/:id/versions", httptransport.Wrap(handler.History))
	contributor := api.Group("", httptransport.RequireRoles(httptransport.RoleAnalyst, httptransport.RoleEditor, httptransport.RoleAdmin))
	contributor.POST("", httptransport.Wrap(handler.Create))
	contributor.PUT("/:id", httptransport.Wrap(handler.Update))
	contributor.PUT("/:id/draft", httptransport.Wrap(handler.ReplaceDraft))
	contributor.POST("/:id/preview", httptransport.Wrap(handler.Preview))
	contributor.POST("/:id/publish", httptransport.Wrap(handler.Publish))
	contributor.POST("/:id/pause", httptransport.Wrap(handler.Pause))
	contributor.POST("/:id/resume", httptransport.Wrap(handler.Resume))
	contributor.POST("/:id/archive", httptransport.Wrap(handler.Archive))
	contributor.POST("/:id/restore", httptransport.Wrap(handler.Restore))
	reviewer := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	reviewer.POST("/:id/draft/rules/:rule_id/approval", httptransport.Wrap(handler.ApproveAICandidate))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/:id/draft/ai-candidates", httptransport.Wrap(handler.AddAICandidate))
	admin.DELETE("/:id", httptransport.Wrap(handler.Delete))
}

func RegisterAgentRoutes(router *gin.Engine, service monitorService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/agent/monitors", httptransport.RequireAuthentication(authenticator), httptransport.RequireAgentScope("monitors.read"))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
}
