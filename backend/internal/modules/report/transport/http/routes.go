package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service reportService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/reports", httptransport.RequireAuthentication(authenticator))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
	api.POST("/:id/preview", httptransport.Wrap(handler.Preview))
	contributor := api.Group("", httptransport.RequireRoles(httptransport.RoleAnalyst, httptransport.RoleEditor, httptransport.RoleAdmin))
	contributor.POST("", httptransport.Wrap(handler.Create))
	contributor.POST("/:id/build", httptransport.Wrap(handler.Build))
	contributor.POST("/:id/submit", httptransport.Wrap(handler.SubmitForApproval))
	approver := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	approver.POST("/:id/approve", httptransport.Wrap(handler.ApproveRevision))
	approver.POST("/:id/reject", httptransport.Wrap(handler.RejectRevision))
}

func RegisterAgentRoutes(router *gin.Engine, service reportService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	api := router.Group("/api/v1/agent/reports", httptransport.RequireAuthentication(authenticator), httptransport.RequireAgentScope("reports.read"))
	api.GET("", httptransport.Wrap(handler.List))
	api.GET("/:id", httptransport.Wrap(handler.Get))
}
