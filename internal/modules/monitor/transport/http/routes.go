package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
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
	editor := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	editor.POST("", httptransport.Wrap(handler.Create))
	editor.PUT("/:id/draft", httptransport.Wrap(handler.ReplaceDraft))
	editor.POST("/:id/preview", httptransport.Wrap(handler.Preview))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/:id/draft/ai-candidates", httptransport.Wrap(handler.AddAICandidate))
	admin.POST("/:id/draft/rules/:rule_id/approval", httptransport.Wrap(handler.ApproveAICandidate))
	admin.POST("/:id/publish", httptransport.Wrap(handler.Publish))
	admin.POST("/:id/pause", httptransport.Wrap(handler.Pause))
	admin.POST("/:id/resume", httptransport.Wrap(handler.Resume))
	admin.POST("/:id/archive", httptransport.Wrap(handler.Archive))
	admin.POST("/:id/restore", httptransport.Wrap(handler.Restore))
	admin.DELETE("/:id", httptransport.Wrap(handler.Delete))
}
