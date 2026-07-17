package http

import (
	"context"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	domain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type overviewService interface {
	Get(context.Context) (domain.RuntimeOverview, error)
}

type OverviewHandler struct{ service overviewService }

func NewOverviewHandler(service overviewService) *OverviewHandler {
	return &OverviewHandler{service: service}
}

func RegisterOverviewRoutes(router *gin.Engine, service *operationsapplication.OverviewService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewOverviewHandler(service)
	admin := router.Group("/api/v1/operations/overview", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("", httptransport.Wrap(handler.Get))
}

// Get returns queue counters and the oldest available schedule time.
// @Summary Get runtime overview
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Success 200 {object} OverviewResult[domain.RuntimeOverview]
// @Failure 401 {object} OverviewResult[EmptyResponse]
// @Failure 403 {object} OverviewResult[EmptyResponse]
// @Failure 503 {object} OverviewResult[EmptyResponse]
// @Router /api/v1/operations/overview [get]
func (handler *OverviewHandler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	overview, err := handler.service.Get(c.Request.Context())
	if err != nil {
		return operationsapplication.JobHTTPError(err)
	}
	httptransport.OK(c, overview)
	return nil
}
