package http

import (
	application "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, authenticator httptransport.Authenticator) {
	registerRoutes(router, read, lifecycle, governance, nil, nil, nil, nil, nil, authenticator)
}

func RegisterRoutesWithHeat(router *gin.Engine, read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService, authenticator httptransport.Authenticator) {
	registerRoutes(router, read, lifecycle, governance, heat, nil, nil, nil, nil, authenticator)
}

func RegisterRoutesWithHeatAndClaims(router *gin.Engine, read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService, claims *application.ClaimService, authenticator httptransport.Authenticator) {
	registerRoutes(router, read, lifecycle, governance, heat, claims, nil, nil, nil, authenticator)
}

func RegisterRoutesWithIntelligence(router *gin.Engine, read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService, claims *application.ClaimService, intelligence *application.EventIntelligenceReadService, summaries *application.EventSummaryService, extractions *application.EventClaimExtractionService, authenticator httptransport.Authenticator) {
	registerRoutes(router, read, lifecycle, governance, heat, claims, intelligence, summaries, extractions, authenticator)
}

func registerRoutes(router *gin.Engine, read *application.ReadService, lifecycle *application.LifecycleService, governance *application.GovernanceService, heat *application.HeatService, claims *application.ClaimService, intelligence *application.EventIntelligenceReadService, summaries *application.EventSummaryService, extractions *application.EventClaimExtractionService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewHandlerWithIntelligence(read, lifecycle, governance, heat, claims, intelligence, summaries, extractions)
	api := router.Group("/api/v1/events", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.GET("/:id/contents", httptransport.Wrap(handler.ListMembers))
	if heat != nil {
		api.GET("/:id/heat", httptransport.Wrap(handler.GetHeat))
	}
	if intelligence != nil {
		api.GET("/:id/intelligence", httptransport.Wrap(handler.GetIntelligence))
	}
	if claims != nil {
		claimAdmin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin, httptransport.RoleEditor))
		claimAdmin.POST("/:id/claims", httptransport.Wrap(handler.SaveClaim))
	}
	editor := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	editor.POST("/:id/contents/:content_id/lock", httptransport.Wrap(handler.SetMemberLock))
	if summaries != nil {
		editor.POST("/:id/intelligence/summary/regenerate", httptransport.Wrap(handler.RegenerateSummary))
	}
	if extractions != nil {
		editor.POST("/:id/intelligence/extract", httptransport.Wrap(handler.RegenerateExtraction))
	}
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/:id/lifecycle", httptransport.Wrap(handler.Transition))
	admin.POST("/:id/merge", httptransport.Wrap(handler.Merge))
	admin.POST("/:id/split", httptransport.Wrap(handler.Split))
}

func RegisterRadarRoutes(router *gin.Engine, radar *application.RadarService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := &Handler{radar: radar}
	api := router.Group("/api/v1/radar", httptransport.RequireAuthentication(authenticator))
	api.GET("/events", httptransport.Wrap(handler.ListRadar))
}

func RegisterEventUpdateRoutes(router *gin.Engine, updates *application.UpdateService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := &Handler{updates: updates}
	api := router.Group("/api/v1/events", httptransport.RequireAuthentication(authenticator))
	api.GET("/:id/updates", httptransport.Wrap(handler.ListUpdates))
}

func RegisterAgentRoutesWithIntelligence(router *gin.Engine, read *application.ReadService, heat *application.HeatService, intelligence *application.EventIntelligenceReadService, authenticator httptransport.Authenticator) {
	if router == nil || read == nil {
		return
	}
	handler := NewHandlerWithIntelligence(read, nil, nil, heat, nil, intelligence, nil, nil)
	api := router.Group("/api/v1/agent/events", httptransport.RequireAuthentication(authenticator), httptransport.RequireAgentScope("events.read"))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.GET("/:id/contents", httptransport.Wrap(handler.ListMembers))
	if heat != nil {
		api.GET("/:id/heat", httptransport.Wrap(handler.GetHeat))
	}
	if intelligence != nil {
		api.GET("/:id/intelligence", httptransport.Wrap(handler.GetIntelligence))
	}
}

func RegisterAgentRadarRoutes(router *gin.Engine, radar *application.RadarService, authenticator httptransport.Authenticator) {
	if router == nil || radar == nil {
		return
	}
	handler := &Handler{radar: radar}
	api := router.Group("/api/v1/agent/radar", httptransport.RequireAuthentication(authenticator), httptransport.RequireAgentScope("events.read"))
	api.GET("/events", httptransport.Wrap(handler.ListRadar))
}

func RegisterAgentEventUpdateRoutes(router *gin.Engine, updates *application.UpdateService, authenticator httptransport.Authenticator) {
	if router == nil || updates == nil {
		return
	}
	handler := &Handler{updates: updates}
	api := router.Group("/api/v1/agent/events", httptransport.RequireAuthentication(authenticator), httptransport.RequireAgentScope("events.read"))
	api.GET("/:id/updates", httptransport.Wrap(handler.ListUpdates))
}
