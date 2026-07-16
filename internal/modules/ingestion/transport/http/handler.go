package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

const (
	contentQueryListOperation = "list_active"
	contentQueryGetOperation  = "get_active"
)

// contentQueryService is the ingestion application boundary used by this
// transport. It intentionally exposes neither a repository nor evidence
// store, making object download and storage configuration inaccessible here.
type contentQueryService interface {
	ListActive(context.Context, ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error)
	GetActive(context.Context, int64) (ingestiondomain.Content, error)
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
// @Param limit query int false "page size"
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

func contentListQuery(c *gin.Context) (ingestiondomain.ContentListQuery, error) {
	query := ingestiondomain.ContentListQuery{Cursor: c.Query("cursor")}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return ingestiondomain.ContentListQuery{}, invalidRequest(fmt.Errorf("invalid content limit"))
		}
		query.Limit = limit
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
