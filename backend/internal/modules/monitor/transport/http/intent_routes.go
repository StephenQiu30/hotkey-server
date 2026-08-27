package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterIntentRoutes mounts the independent semantic Intent resource. The
// legacy monitor service remains untouched and cannot stand in for this API.
func RegisterIntentRoutes(router *gin.Engine, service intentHTTPService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewIntentHandler(service)
	api := router.Group("/api/v1/monitors", httptransport.RequireAuthentication(authenticator))
	editor := api.Group("", httptransport.RequireRoles(httptransport.RoleAnalyst, httptransport.RoleEditor, httptransport.RoleAdmin))
	editor.GET("/:id/draft", httptransport.Wrap(handler.GetDraft))
	editor.PUT("/:id/draft/intent", httptransport.Wrap(handler.PutDraft))
	editor.POST("/:id/draft/preview-runs", httptransport.Wrap(handler.SubmitPreviewRun))
	editor.GET("/:id/draft/preview-runs/:run_id", httptransport.Wrap(handler.GetPreviewRun))
	editor.GET("/:id/draft/expansion-runs/:run_id", httptransport.Wrap(handler.GetExpansionRun))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/:id/draft/expansion-runs", httptransport.Wrap(handler.SubmitExpansionRun))
	admin.POST("/:id/draft/expansion-candidates/:candidate_id/decision", httptransport.Wrap(handler.ReviewExpansionCandidate))
}
