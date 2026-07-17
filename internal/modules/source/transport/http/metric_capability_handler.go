package http

import (
	"context"
	"fmt"
	"strconv"

	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type metricCapabilityService interface {
	CreateDraft(context.Context, sourceapplication.CreateMetricCapabilityInput) (*domain.MetricCapabilityProfile, error)
	Publish(context.Context, sourceapplication.MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error)
	Archive(context.Context, sourceapplication.MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error)
}

type MetricCapabilityHandler struct{ service metricCapabilityService }

func NewMetricCapabilityHandler(service metricCapabilityService) *MetricCapabilityHandler {
	return &MetricCapabilityHandler{service: service}
}

// CreateDraft creates an administrator-managed source capability draft.
// @Summary Create a metric capability profile draft
// @Tags source metric capabilities
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateMetricCapabilityProfileRequest true "metric capability profile"
// @Success 201 {object} SourceResult[MetricCapabilityProfileResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/metric-capability-profiles [post]
func (handler *MetricCapabilityHandler) CreateDraft(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	var request CreateMetricCapabilityProfileRequest
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	profile, err := handler.service.CreateDraft(c.Request.Context(), sourceapplication.CreateMetricCapabilityInput{Subject: subject, Profile: metricCapabilityProfile(request)})
	if err != nil {
		return err
	}
	httptransport.Created(c, metricCapabilityProfileResponse(*profile))
	return nil
}

// Publish activates a draft and archives the prior source-type profile in the
// same application transaction.
// @Summary Publish a metric capability profile
// @Tags source metric capabilities
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "metric capability profile ID"
// @Param request body MetricCapabilityLifecycleRequest true "expected version and reason"
// @Success 200 {object} SourceResult[MetricCapabilityProfileResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/metric-capability-profiles/{id}/publish [post]
func (handler *MetricCapabilityHandler) Publish(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Publish)
}

// Archive stops a profile from being selected while retaining its immutable
// historical configuration and snapshots.
// @Summary Archive a metric capability profile
// @Tags source metric capabilities
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "metric capability profile ID"
// @Param request body MetricCapabilityLifecycleRequest true "expected version and reason"
// @Success 200 {object} SourceResult[MetricCapabilityProfileResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Failure 403 {object} SourceResult[EmptyResponse]
// @Failure 404 {object} SourceResult[EmptyResponse]
// @Failure 409 {object} SourceResult[EmptyResponse]
// @Failure 503 {object} SourceResult[EmptyResponse]
// @Router /api/v1/metric-capability-profiles/{id}/archive [post]
func (handler *MetricCapabilityHandler) Archive(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Archive)
}

func (handler *MetricCapabilityHandler) lifecycle(c *gin.Context, operation func(context.Context, sourceapplication.MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error)) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := metricCapabilityProfileID(c)
	if err != nil {
		return err
	}
	var request MetricCapabilityLifecycleRequest
	if err := bindStrictJSON(c, &request); err != nil {
		return invalidRequest(err)
	}
	profile, err := operation(c.Request.Context(), sourceapplication.MetricCapabilityLifecycleInput{Subject: subject, ID: id, ExpectedVersion: request.ExpectedVersion, ReasonCode: request.ReasonCode})
	if err != nil {
		return err
	}
	httptransport.OK(c, metricCapabilityProfileResponse(*profile))
	return nil
}

func metricCapabilityProfile(request CreateMetricCapabilityProfileRequest) domain.MetricCapabilityProfile {
	return domain.MetricCapabilityProfile{
		SourceType: domain.SourceType(request.SourceType), ProfileVersion: request.ProfileVersion,
		SupportsViews: request.SupportsViews, SupportsLikes: request.SupportsLikes, SupportsComments: request.SupportsComments,
		SupportsShares: request.SupportsShares, IndependenceStrategy: domain.IndependenceStrategy(request.IndependenceStrategy),
		NormalizationWindowHours: request.NormalizationWindowHours, CredibilityWeight: request.CredibilityWeight,
		MaxSingleItemContribution: request.MaxSingleItemContribution,
	}
}

func metricCapabilityProfileID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid metric capability profile id"))
	}
	return id, nil
}
