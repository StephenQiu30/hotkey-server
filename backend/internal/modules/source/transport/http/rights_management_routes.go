package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// RegisterRightsManagementRoutes is intentionally independent from bootstrap
// wiring so the canonical router can mount this capability without a legacy
// /sources alias or a second route source of truth.
func RegisterRightsManagementRoutes(router *gin.Engine, service rightsManagementHTTPService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewRightsManagementHandler(service)
	endpoint := router.Group("/api/v1/source-endpoints/:id", rightsManagementResponseHeaders(), httptransport.RequireAuthentication(authenticator))
	endpoint.GET("/capabilities", httptransport.Wrap(handler.GetCapability))

	admin := endpoint.Group("", httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("/rights-policies", httptransport.Wrap(handler.ListPolicies))
	admin.POST("/rights-policies", httptransport.Wrap(handler.CreatePolicy))
	admin.GET("/rights-decision-batches", httptransport.Wrap(handler.ListDecisionBatches))
	admin.POST("/rights-decision-batches", httptransport.Wrap(handler.RecordDecisionBatch))
	admin.GET("/rights-decisions/:decision_id", httptransport.Wrap(handler.GetDecision))
	admin.POST("/rights-evaluations", httptransport.Wrap(handler.EvaluateActions))
}

func rightsManagementResponseHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		prepareRightsManagementResponse(c)
		c.Next()
	}
}
