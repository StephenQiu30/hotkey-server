package http

import (
	"context"
	"strconv"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type collectionControlService interface {
	List(context.Context, sourceapplication.CollectionRunListInput) (domain.CollectionRunPage, error)
	Manual(context.Context, sourceapplication.ManualCollectionInput) (domain.ManualCollectionSummary, error)
	Scans(context.Context, sourceapplication.MonitorScanListInput) ([]domain.MonitorScan, error)
	Retry(context.Context, sourceapplication.CollectionRunRetryInput) (domain.CollectionRunSummary, error)
	Health(context.Context, sourceapplication.SourceHealthInput) (domain.SourceHealth, error)
}

// Scans lists recent runs for one Monitor without exposing connector request
// internals.
// @Summary List recent monitor scans
// @Tags collection-runs
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param limit query int false "scan count" default(20) minimum(1) maximum(100)
// @Success 200 {object} CollectionResult[MonitorScanPageResponse]
// @Failure 400 {object} CollectionResult[EmptyResponse]
// @Failure 401 {object} CollectionResult[EmptyResponse]
// @Failure 503 {object} CollectionResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/scans [get]
func (handler *CollectionHandler) Scans(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := collectionResourceID(c, "monitor")
	if err != nil {
		return err
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return domain.InvalidCollectionRequest()
		}
	}
	items, err := handler.service.Scans(c.Request.Context(), sourceapplication.MonitorScanListInput{Subject: subject, MonitorID: id, Limit: limit})
	if err != nil {
		return err
	}
	httptransport.OK(c, monitorScanPageResponse(items))
	return nil
}

// Manual submits an immediate durable collection for an active published
// Monitor. The request cannot supply a query, source, window or connector.
// @Summary Trigger an immediate monitor collection
// @Tags collection-runs
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} CollectionResult[ManualCollectionResponse]
// @Failure 400 {object} CollectionResult[EmptyResponse]
// @Failure 401 {object} CollectionResult[EmptyResponse]
// @Failure 403 {object} CollectionResult[EmptyResponse]
// @Failure 409 {object} CollectionResult[EmptyResponse]
// @Failure 503 {object} CollectionResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/collect [post]
func (handler *CollectionHandler) Manual(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	subject, err := sourceSubject(c)
	if err != nil {
		return err
	}
	id, err := collectionResourceID(c, "monitor")
	if err != nil {
		return err
	}
	summary, err := handler.service.Manual(c.Request.Context(), sourceapplication.ManualCollectionInput{Subject: subject, MonitorID: id})
	if err != nil {
		return err
	}
	httptransport.OK(c, manualCollectionResponse(summary))
	return nil
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
