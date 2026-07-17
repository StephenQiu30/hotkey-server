package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func RegisterMetricCapabilityRoutes(router *gin.Engine, service metricCapabilityService, authenticator httptransport.Authenticator) {
	if router == nil {
		return
	}
	handler := NewMetricCapabilityHandler(service)
	api := router.Group("/api/v1/metric-capability-profiles", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	api.POST("", httptransport.Wrap(handler.CreateDraft))
	api.POST("/:id/publish", httptransport.Wrap(handler.Publish))
	api.POST("/:id/archive", httptransport.Wrap(handler.Archive))
}
