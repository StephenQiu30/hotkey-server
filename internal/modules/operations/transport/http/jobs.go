package http

import (
	"context"
	"fmt"
	"strconv"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type jobService interface {
	List(context.Context, operationsdomain.JobListQuery) (operationsdomain.JobPage, error)
	Cancel(context.Context, operationsdomain.JobMutationInput) (operationsdomain.JobSummary, error)
	Retry(context.Context, operationsdomain.JobMutationInput) (operationsdomain.JobSummary, error)
}

type JobsHandler struct{ service jobService }

func NewJobsHandler(service jobService) *JobsHandler { return &JobsHandler{service: service} }

func RegisterJobRoutes(router *gin.Engine, service *operationsapplication.JobService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewJobsHandler(service)
	admin := router.Group("/api/v1/operations/jobs", httptransport.RequireAuthentication(authenticator), httptransport.RequireRoles(httptransport.RoleAdmin))
	admin.GET("", httptransport.Wrap(handler.List))
	admin.POST("/:id/cancel", httptransport.Wrap(handler.Cancel))
	admin.POST("/:id/retry", httptransport.Wrap(handler.Retry))
}

// List exposes only safe River job metadata; args, errors, provider payloads
// and credentials never cross the Operations transport boundary.
// @Summary List durable jobs
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Param cursor query int false "last job id"
// @Param kind query string false "job kind"
// @Param state query string false "job state"
// @Param limit query int false "page size"
// @Success 200 {object} JobResult[JobPageResponse]
// @Failure 400 {object} JobResult[EmptyResponse]
// @Failure 401 {object} JobResult[EmptyResponse]
// @Failure 403 {object} JobResult[EmptyResponse]
// @Failure 503 {object} JobResult[EmptyResponse]
// @Router /api/v1/operations/jobs [get]
func (handler *JobsHandler) List(c *gin.Context) error {
	httptransport.SetModule(c, "operations")
	query, err := parseJobListQuery(c)
	if err != nil {
		return operationsapplication.JobHTTPError(err)
	}
	page, err := handler.service.List(c.Request.Context(), query)
	if err != nil {
		return operationsapplication.JobHTTPError(err)
	}
	httptransport.OK(c, jobPageResponse(page))
	return nil
}

// Cancel marks an available job cancelled without exposing its args.
// @Summary Cancel a durable job
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Param id path int true "job id"
// @Success 200 {object} JobResult[JobResponse]
// @Failure 400 {object} JobResult[EmptyResponse]
// @Failure 401 {object} JobResult[EmptyResponse]
// @Failure 403 {object} JobResult[EmptyResponse]
// @Failure 404 {object} JobResult[EmptyResponse]
// @Failure 409 {object} JobResult[EmptyResponse]
// @Failure 503 {object} JobResult[EmptyResponse]
// @Router /api/v1/operations/jobs/{id}/cancel [post]
func (handler *JobsHandler) Cancel(c *gin.Context) error {
	return handler.mutate(c, false)
}

// Retry requeues only a discarded or cancelled job and resets its attempt
// counter; it never accepts a caller-supplied payload.
// @Summary Retry a durable job
// @Tags operations
// @Produce json
// @Security BearerAuth
// @Param id path int true "job id"
// @Success 200 {object} JobResult[JobResponse]
// @Failure 400 {object} JobResult[EmptyResponse]
// @Failure 401 {object} JobResult[EmptyResponse]
// @Failure 403 {object} JobResult[EmptyResponse]
// @Failure 404 {object} JobResult[EmptyResponse]
// @Failure 409 {object} JobResult[EmptyResponse]
// @Failure 503 {object} JobResult[EmptyResponse]
// @Router /api/v1/operations/jobs/{id}/retry [post]
func (handler *JobsHandler) Retry(c *gin.Context) error {
	return handler.mutate(c, true)
}

func (handler *JobsHandler) mutate(c *gin.Context, retry bool) error {
	httptransport.SetModule(c, "operations")
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return operationsapplication.JobHTTPError(context.Canceled)
	}
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || jobID <= 0 {
		return operationsapplication.JobHTTPError(fmt.Errorf("invalid job id"))
	}
	input := operationsdomain.JobMutationInput{ActorID: subject.UserID, JobID: jobID}
	var job operationsdomain.JobSummary
	if retry {
		job, err = handler.service.Retry(c.Request.Context(), input)
	} else {
		job, err = handler.service.Cancel(c.Request.Context(), input)
	}
	if err != nil {
		return operationsapplication.JobHTTPError(err)
	}
	httptransport.OK(c, jobResponse(job))
	return nil
}

func parseJobListQuery(c *gin.Context) (operationsdomain.JobListQuery, error) {
	query := operationsdomain.JobListQuery{Kind: c.Query("kind")}
	if raw := c.Query("state"); raw != "" {
		query.State = operationsdomain.JobState(raw)
	}
	if raw := c.Query("cursor"); raw != "" {
		cursor, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return operationsdomain.JobListQuery{}, err
		}
		query.Cursor = cursor
	}
	query.Limit = 50
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return operationsdomain.JobListQuery{}, err
		}
		query.Limit = limit
	}
	return query, nil
}
