package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, handler *Handler, authenticator httptransport.Authenticator) {
	if router == nil || handler == nil {
		return
	}
	api := router.Group("/api/v1/knowledge", httptransport.RequireAuthentication(authenticator))
	publisher := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	publisher.GET("/documents", httptransport.Wrap(handler.ListDocuments))
	publisher.GET("/documents/:id", httptransport.Wrap(handler.GetDocument))
	publisher.GET("/proposals", httptransport.Wrap(handler.ListProposals))
	publisher.GET("/proposals/:id", httptransport.Wrap(handler.GetProposal))
	publisher.POST("/proposals", httptransport.Wrap(handler.Create))
	publisher.POST("/proposals/:id/approve", httptransport.Wrap(handler.Approve))
	publisher.POST("/proposals/:id/reject", httptransport.Wrap(handler.Reject))
	publisher.POST("/proposals/:id/apply", httptransport.Wrap(handler.Apply))
	admin := api.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.POST("/reconcile", httptransport.Wrap(handler.Reconcile))
}
