package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"

	reportapplication "github.com/StephenQiu30/hotkey-server/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type reportService interface {
	List(context.Context, domain.ListQuery) (domain.Page, error)
	Get(context.Context, int64) (domain.Report, error)
	Preview(context.Context, int64) (domain.Report, error)
	Publish(context.Context, int64) (domain.Report, error)
}

// Build creates or refreshes a deterministic draft from the current event
// projection. Published reports remain immutable and return a conflict.
// @Summary Build a report draft
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "report ID"
// @Success 200 {object} ReportResult[ReportResponse]
// @Failure 400 {object} ReportResult[EmptyResponse]
// @Failure 401 {object} ReportResult[EmptyResponse]
// @Failure 403 {object} ReportResult[EmptyResponse]
// @Failure 404 {object} ReportResult[EmptyResponse]
// @Failure 409 {object} ReportResult[EmptyResponse]
// @Failure 503 {object} ReportResult[EmptyResponse]
// @Router /api/v1/reports/{id}/build [post]
func (handler *Handler) Build(c *gin.Context) error {
	httptransport.SetModule(c, "report")
	reportID, err := reportID(c)
	if err != nil {
		return err
	}
	builder, ok := handler.service.(interface {
		BuildByID(context.Context, int64) (domain.Report, error)
	})
	if !ok {
		return reportError(sharedrepository.ErrUnavailable)
	}
	report, err := builder.BuildByID(c.Request.Context(), reportID)
	if err != nil {
		return reportError(err)
	}
	httptransport.OK(c, reportResponse(report))
	return nil
}

var _ reportService = (*reportapplication.Service)(nil)

type Handler struct{ service reportService }

func NewHandler(service reportService) *Handler { return &Handler{service: service} }

// List returns report metadata in reverse creation order. Details and frozen
// snapshots are fetched through the report detail endpoint.
// @Summary List reports
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param cursor query int false "report id cursor"
// @Param limit query int false "page size"
// @Param type query string false "daily or weekly"
// @Param status query string false "draft, published, failed or archived"
// @Success 200 {object} ReportResult[ReportPageResponse]
// @Failure 400 {object} ReportResult[EmptyResponse]
// @Failure 401 {object} ReportResult[EmptyResponse]
// @Failure 503 {object} ReportResult[EmptyResponse]
// @Router /api/v1/reports [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "report")
	query, err := reportListQuery(c)
	if err != nil {
		return err
	}
	page, err := handler.service.List(c.Request.Context(), query)
	if err != nil {
		return reportError(err)
	}
	response := ReportPageResponse{Items: make([]ReportResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, report := range page.Items {
		response.Items = append(response.Items, reportResponse(report))
	}
	httptransport.OK(c, response)
	return nil
}

// Get returns a report and every persisted snapshot item. Published reports
// are always returned from their frozen database representation.
// @Summary Get a report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "report ID"
// @Success 200 {object} ReportResult[ReportResponse]
// @Failure 400 {object} ReportResult[EmptyResponse]
// @Failure 401 {object} ReportResult[EmptyResponse]
// @Failure 404 {object} ReportResult[EmptyResponse]
// @Failure 503 {object} ReportResult[EmptyResponse]
// @Router /api/v1/reports/{id} [get]
func (handler *Handler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "report")
	reportID, err := reportID(c)
	if err != nil {
		return err
	}
	report, err := handler.service.Get(c.Request.Context(), reportID)
	if err != nil {
		return reportError(err)
	}
	httptransport.OK(c, reportResponse(report))
	return nil
}

// Preview returns exactly the stored report snapshot without mutating status.
// @Summary Preview a report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "report ID"
// @Success 200 {object} ReportResult[ReportPreviewResponse]
// @Failure 400 {object} ReportResult[EmptyResponse]
// @Failure 401 {object} ReportResult[EmptyResponse]
// @Failure 404 {object} ReportResult[EmptyResponse]
// @Failure 503 {object} ReportResult[EmptyResponse]
// @Router /api/v1/reports/{id}/preview [post]
func (handler *Handler) Preview(c *gin.Context) error {
	httptransport.SetModule(c, "report")
	reportID, err := reportID(c)
	if err != nil {
		return err
	}
	report, err := handler.service.Preview(c.Request.Context(), reportID)
	if err != nil {
		return reportError(err)
	}
	httptransport.OK(c, ReportPreviewResponse{Report: reportResponse(report), Publishable: report.Status == domain.ReportDraft})
	return nil
}

// Publish freezes a draft report. Repeating this request is a 409 rather than
// a silent rewrite, preserving the snapshot contract for downstream delivery.
// @Summary Publish a draft report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "report ID"
// @Success 200 {object} ReportResult[ReportResponse]
// @Failure 400 {object} ReportResult[EmptyResponse]
// @Failure 401 {object} ReportResult[EmptyResponse]
// @Failure 403 {object} ReportResult[EmptyResponse]
// @Failure 404 {object} ReportResult[EmptyResponse]
// @Failure 409 {object} ReportResult[EmptyResponse]
// @Failure 503 {object} ReportResult[EmptyResponse]
// @Router /api/v1/reports/{id}/publish [post]
func (handler *Handler) Publish(c *gin.Context) error {
	httptransport.SetModule(c, "report")
	reportID, err := reportID(c)
	if err != nil {
		return err
	}
	report, err := handler.service.Publish(c.Request.Context(), reportID)
	if err != nil {
		return reportError(err)
	}
	httptransport.OK(c, reportResponse(report))
	return nil
}

func reportID(c *gin.Context) (int64, error) {
	value, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || value <= 0 {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid report id")
	}
	return value, nil
}

func reportListQuery(c *gin.Context) (domain.ListQuery, error) {
	query := domain.ListQuery{Limit: 20}
	if value := c.Query("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return domain.ListQuery{}, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid report limit")
		}
		query.Limit = limit
	}
	if value := c.Query("cursor"); value != "" {
		cursor, err := strconv.ParseInt(value, 10, 64)
		if err != nil || cursor <= 0 {
			return domain.ListQuery{}, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid report cursor")
		}
		query.Cursor = cursor
	}
	if value := c.Query("type"); value != "" {
		reportType := domain.ReportType(value)
		query.Type = &reportType
	}
	if value := c.Query("status"); value != "" {
		status := domain.ReportStatus(value)
		query.Status = &status
	}
	if err := query.Validate(); err != nil {
		return domain.ListQuery{}, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid report filters")
	}
	return query, nil
}

func reportError(err error) error {
	switch {
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "report not found")
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid report request")
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrImmutable):
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "report is immutable or conflicted")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "report service unavailable")
	default:
		return fmt.Errorf("report operation: %w", err)
	}
}
