package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

const (
	contentQueryListOperation = "list_active"
	contentQueryGetOperation  = "get_active"
	contentDocumentOperation  = "get_document"
	contentDeleteOperation    = "delete_active"
)

// contentQueryService is the ingestion application boundary used by this
// transport. It intentionally exposes neither a repository nor evidence
// store, making object download and storage configuration inaccessible here.
type contentQueryService interface {
	ListActive(context.Context, ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error)
	GetActive(context.Context, int64) (ingestiondomain.Content, error)
	GetDocument(context.Context, int64) (ingestiondomain.ContentDocument, error)
	DeleteContent(context.Context, int64) (ingestionapplication.DeleteBySourceItemResult, error)
}

type Handler struct {
	service contentQueryService
	metrics *observability.Metrics
}

func NewHandler(service contentQueryService, metrics *observability.Metrics) *Handler {
	return &Handler{service: service, metrics: metrics}
}

// List returns only active, safe Content projections.
// @Summary List active content
// @Tags contents
// @Produce json
// @Security BearerAuth
// @Param cursor query string false "cursor"
// @Param limit query int false "page size" minimum(1) maximum(100)
// @Param q query string false "title or summary keyword" maxlength(100)
// @Param source_connection_id query int false "source connection ID"
// @Param published_from query string false "published at or after (RFC3339)"
// @Param published_to query string false "published at or before (RFC3339)"
// @Param monitor_id query int false "monitor ID"
// @Param decision query string false "latest monitor match decision" Enums(accepted,review,rejected)
// @Param sort query string false "sort order" Enums(latest,relevance)
// @Success 200 {object} ContentResult[ContentPageResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/contents [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	query, err := contentListQuery(c)
	if err != nil {
		handler.record(contentQueryListOperation, err)
		return err
	}
	page, err := handler.service.ListActive(c.Request.Context(), query)
	if err != nil {
		handler.record(contentQueryListOperation, err)
		return err
	}
	handler.record(contentQueryListOperation, nil)
	httptransport.OK(c, contentPageResponse(page))
	return nil
}

// Get returns one active, safe Content projection.
// @Summary Get active content
// @Tags contents
// @Produce json
// @Security BearerAuth
// @Param id path int true "content ID"
// @Success 200 {object} ContentResult[ContentResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/contents/{id} [get]
func (handler *Handler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	id, err := contentID(c)
	if err != nil {
		handler.record(contentQueryGetOperation, err)
		return err
	}
	content, err := handler.service.GetActive(c.Request.Context(), id)
	if err != nil {
		handler.record(contentQueryGetOperation, err)
		return err
	}
	handler.record(contentQueryGetOperation, nil)
	httptransport.OK(c, contentResponse(content))
	return nil
}

// Document returns the newest verified Markdown projection for one active Content.
// @Summary Get captured content Markdown document
// @Description Returns a successful safe projection for ready, not_captured, and unavailable evidence states; Markdown is present only when verified.
// @Tags contents
// @Produce json
// @Security BearerAuth
// @Param id path int true "content ID"
// @Success 200 {object} ContentResult[ContentDocumentResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/contents/{id}/document [get]
func (handler *Handler) Document(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	id, err := contentID(c)
	if err != nil {
		handler.record(contentDocumentOperation, err)
		return err
	}
	document, err := handler.service.GetDocument(c.Request.Context(), id)
	if err != nil {
		handler.record(contentDocumentOperation, err)
		return err
	}
	handler.record(contentDocumentOperation, nil)
	httptransport.OK(c, contentDocumentResponse(document))
	return nil
}

// Delete removes an active Content from operational views and schedules its
// deterministic evidence objects for deletion. The database row remains as a
// tombstone so source replay, downstream history, and lifecycle retries stay
// auditable.
// @Summary Delete active content
// @Description Soft-deletes one fetched Content and removes its archived evidence when available.
// @Tags contents
// @Produce json
// @Security BearerAuth
// @Param id path int true "content ID"
// @Success 200 {object} ContentResult[EmptyResponse]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/contents/{id} [delete]
func (handler *Handler) Delete(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	id, err := contentID(c)
	if err != nil {
		handler.record(contentDeleteOperation, err)
		return err
	}
	if _, err := handler.service.DeleteContent(c.Request.Context(), id); err != nil {
		handler.record(contentDeleteOperation, err)
		return err
	}
	handler.record(contentDeleteOperation, nil)
	httptransport.Empty(c)
	return nil
}

func contentListQuery(c *gin.Context) (ingestiondomain.ContentListQuery, error) {
	query := ingestiondomain.ContentListQuery{
		Cursor: c.Query("cursor"), Keyword: strings.TrimSpace(c.Query("q")),
		Sort: ingestiondomain.ContentSort(c.Query("sort")),
	}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > 100 {
			return ingestiondomain.ContentListQuery{}, invalidRequest(fmt.Errorf("invalid content limit"))
		}
		query.Limit = limit
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	parseID := func(name string) (*int64, error) {
		raw := c.Query(name)
		if raw == "" {
			return nil, nil
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return nil, invalidRequest(fmt.Errorf("invalid content %s", name))
		}
		return &value, nil
	}
	var err error
	if query.SourceConnectionID, err = parseID("source_connection_id"); err != nil {
		return ingestiondomain.ContentListQuery{}, err
	}
	if query.MonitorID, err = parseID("monitor_id"); err != nil {
		return ingestiondomain.ContentListQuery{}, err
	}
	parseTime := func(name string) (*time.Time, error) {
		raw := c.Query(name)
		if raw == "" {
			return nil, nil
		}
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, invalidRequest(fmt.Errorf("invalid content %s", name))
		}
		value = value.UTC()
		return &value, nil
	}
	if query.PublishedFrom, err = parseTime("published_from"); err != nil {
		return ingestiondomain.ContentListQuery{}, err
	}
	if query.PublishedTo, err = parseTime("published_to"); err != nil {
		return ingestiondomain.ContentListQuery{}, err
	}
	if raw := c.Query("decision"); raw != "" {
		value := ingestiondomain.MatchDecision(raw)
		query.Decision = &value
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return ingestiondomain.ContentListQuery{}, invalidRequest(err)
	}
	return query, nil
}

func contentID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidRequest(fmt.Errorf("invalid content id"))
	}
	return id, nil
}

func invalidRequest(cause error) error {
	return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", cause)
}

func (handler *Handler) record(operation string, err error) {
	if handler == nil || handler.metrics == nil {
		return
	}
	handler.metrics.RecordContentQuery(operation, contentQueryOutcome(err))
}

func contentQueryOutcome(err error) string {
	if err == nil {
		return "success"
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		switch appError.Code {
		case sharederrors.CodeInvalidRequest:
			return "invalid"
		case sharederrors.CodeNotFound:
			return "not_found"
		case sharederrors.CodeUnavailable:
			return "unavailable"
		}
	}
	return "error"
}
