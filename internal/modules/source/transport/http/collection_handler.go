package http

import (
	"context"
	"strconv"

	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type collectionControlService interface {
	List(context.Context, sourceapplication.CollectionRunListInput) (domain.CollectionRunPage, error)
	Retry(context.Context, sourceapplication.CollectionRunRetryInput) (domain.CollectionRunSummary, error)
	Health(context.Context, sourceapplication.SourceHealthInput) (domain.SourceHealth, error)
}

type CollectionHandler struct{ service collectionControlService }

func NewCollectionHandler(service collectionControlService) *CollectionHandler {
	return &CollectionHandler{service: service}
}

// List runs exposes only the safe operational projection; no source identity,
// query, cursor, conditional request state or credential data crosses this
// transport boundary.
// @Summary List collection runs
// @Tags collection-runs
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "cursor"
// @Param limit query int false "page size"
// @Success 200 {object} CollectionResult[CollectionRunPageResponse]
// @Failure 400 {object} CollectionResult[EmptyResponse]
// @Failure 401 {object} CollectionResult[EmptyResponse]
// @Failure 403 {object} CollectionResult[EmptyResponse]
// @Failure 503 {object} CollectionResult[EmptyResponse]
// @Router /api/v1/collection-runs [get]
func (handler *CollectionHandler) List(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	query, err := collectionRunListQuery(c)
	if err != nil {
		return err
	}
	page, err := handler.service.List(c.Request.Context(), sourceapplication.CollectionRunListInput{Subject: subject, Query: query})
	if err != nil {
		return err
	}
	httptransport.OK(c, collectionRunPageResponse(page))
	return nil
}

// Retry transitions only a failed/cancelled run back to queued. The handler
// deliberately does not invoke a connector or create a scheduler job.
// @Summary Requeue a failed collection run
// @Tags collection-runs
// @Produce json
// @Security BearerAuth
// @Param id path int true "collection run ID"
// @Success 200 {object} CollectionResult[CollectionRunResponse]
// @Failure 400 {object} CollectionResult[EmptyResponse]
// @Failure 401 {object} CollectionResult[EmptyResponse]
// @Failure 403 {object} CollectionResult[EmptyResponse]
// @Failure 404 {object} CollectionResult[EmptyResponse]
// @Failure 409 {object} CollectionResult[EmptyResponse]
// @Failure 503 {object} CollectionResult[EmptyResponse]
// @Router /api/v1/collection-runs/{id}/retry [post]
func (handler *CollectionHandler) Retry(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := collectionResourceID(c, "collection run")
	if err != nil {
		return err
	}
	summary, err := handler.service.Retry(c.Request.Context(), sourceapplication.CollectionRunRetryInput{Subject: subject, ID: id})
	if err != nil {
		return err
	}
	httptransport.OK(c, collectionRunResponse(summary))
	return nil
}

// Health probes the registered connector and returns a safe, stable result.
// @Summary Probe source connection health
// @Tags sources
// @Produce json
// @Security BearerAuth
// @Param id path int true "source connection ID"
// @Success 200 {object} CollectionResult[SourceHealthResponse]
// @Failure 400 {object} CollectionResult[EmptyResponse]
// @Failure 401 {object} CollectionResult[EmptyResponse]
// @Failure 403 {object} CollectionResult[EmptyResponse]
// @Failure 409 {object} CollectionResult[EmptyResponse]
// @Failure 503 {object} CollectionResult[EmptyResponse]
// @Router /api/v1/source-connections/{id}/health [post]
func (handler *CollectionHandler) Health(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := collectionResourceID(c, "source connection")
	if err != nil {
		return err
	}
	result, err := handler.service.Health(c.Request.Context(), sourceapplication.SourceHealthInput{Subject: subject, ID: id})
	if err != nil {
		return err
	}
	httptransport.OK(c, sourceHealthResponse(result))
	return nil
}

func collectionResourceID(c *gin.Context, resource string) (int64, error) {
	value, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || value <= 0 {
		return 0, domain.InvalidCollectionRequest()
	}
	return value, nil
}

func collectionRunListQuery(c *gin.Context) (domain.CollectionRunListQuery, error) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return domain.CollectionRunListQuery{}, domain.InvalidCollectionRequest()
		}
		limit = value
	}
	return domain.CollectionRunListQuery{Cursor: c.Query("cursor"), Limit: limit}, nil
}
