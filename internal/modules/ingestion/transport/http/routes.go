package http

import (
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the complete Content public read contract. Every
// authenticated role may read the same active, safe projection; there is no
// evidence-object or object-download route.
func RegisterRoutes(router *gin.Engine, service contentQueryService, authenticator httptransport.Authenticator, metrics *observability.Metrics) {
	if router == nil {
		return
	}
	handler := NewHandler(service, metrics)
	contents := router.Group("/api/v1/contents", httptransport.RequireAuthentication(authenticator))
	contents.GET("", httptransport.Wrap(handler.List))
	contents.GET("/:id", httptransport.Wrap(handler.Get))
}
