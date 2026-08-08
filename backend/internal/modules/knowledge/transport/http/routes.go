package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, handler *Handler, authenticator httptransport.Authenticator) {
	if router == nil || handler == nil {
		return
	}
	admin := router.Group("/api/v1/knowledge", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("/documents", httptransport.Wrap(handler.ListDocuments))
	admin.GET("/documents/:id", httptransport.Wrap(handler.GetDocument))
	admin.GET("/proposals", httptransport.Wrap(handler.ListProposals))
	admin.GET("/proposals/:id", httptransport.Wrap(handler.GetProposal))
	admin.POST("/proposals", httptransport.Wrap(handler.Create))
	admin.POST("/proposals/:id/approve", httptransport.Wrap(handler.Approve))
	admin.POST("/proposals/:id/reject", httptransport.Wrap(handler.Reject))
	admin.POST("/proposals/:id/apply", httptransport.Wrap(handler.Apply))
	admin.POST("/reconcile", httptransport.Wrap(handler.Reconcile))
}
