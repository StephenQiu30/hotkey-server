package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
)



type ReportService interface {
	Create(ctx context.Context, userID int64, input dto.CreateInput) (dto.Report, error)
	List(ctx context.Context, userID int64, filter dto.ListFilter) ([]dto.Report, int64, error)
	GetByID(ctx context.Context, id, userID int64) (dto.Report, error)
	HTML(ctx context.Context, id, userID int64) (string, error)
	MarkSent(ctx context.Context, id, userID int64) (dto.Report, error)
}

func RegisterReportRoutes(r gin.IRouter, svc ReportService) {
	if svc == nil {
		return
	}
	r.GET("/api/v1/reports", listReportsHandler(svc))
	r.POST("/api/v1/reports", createReportHandler(svc))
	r.GET("/api/v1/reports/:id", getReportHandler(svc))
	r.GET("/api/v1/reports/:id/html", getReportHTMLHandler(svc))
	r.POST("/api/v1/reports/:id/send", sendReportHandler(svc))
}

// createReportHandler godoc
// @Summary Create a report
// @ID create-report
// @Tags reports
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dto.CreateReportRequest true "Report creation payload"
// @Success 201 {object} ReportResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports [post]
func createReportHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "unauthorized")
			return
		}

		var req dto.CreateReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid input")
			return
		}

		input, err := req.ToInput()
		if err != nil {
			platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "")
			return
		}

		item, err := svc.Create(c.Request.Context(), userID, input)
		if err != nil {
			respondReportError(c, err)
			return
		}
		platformhttp.RespondCreated(c, item)
	}
}

// listReportsHandler godoc
// @Summary List reports
// @ID list-reports
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Max results" default(50)
// @Param offset query int false "Offset" default(0)
// @Param report_type query string false "Filter by report type (daily|weekly)"
// @Success 200 {object} ReportListResponse
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports [get]
func listReportsHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "unauthorized")
			return
		}

		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}
		items, total, err := svc.List(c.Request.Context(), userID, dto.ListFilter{
			ReportType: c.Query("report_type"),
			Limit:      limit,
			Offset:     offset,
		})
		if err != nil {
			platformhttp.RespondInternalError(c)
			return
		}
		platformhttp.RespondPage(c, items, offset/limit+1, limit, int(total))
	}
}

// getReportHandler godoc
// @Summary Get a report by ID
// @ID get-report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "Report ID"
// @Success 200 {object} ReportResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports/{id} [get]
func getReportHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "unauthorized")
			return
		}
		id, ok := parseReportID(c)
		if !ok {
			return
		}
		item, err := svc.GetByID(c.Request.Context(), id, userID)
		if err != nil {
			respondReportError(c, err)
			return
		}
		platformhttp.RespondOK(c, item)
	}
}

// getReportHTMLHandler godoc
// @Summary Get report as HTML
// @ID get-report-html
// @Tags reports
// @Produce html
// @Security BearerAuth
// @Param id path int true "Report ID"
// @Success 200 {string} string "HTML content"
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports/{id}/html [get]
func getReportHTMLHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "unauthorized")
			return
		}
		id, ok := parseReportID(c)
		if !ok {
			return
		}
		html, err := svc.HTML(c.Request.Context(), id, userID)
		if err != nil {
			respondReportError(c, err)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}

// sendReportHandler godoc
// @Summary Mark and send a report
// @ID send-report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "Report ID"
// @Success 200 {object} ReportResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports/{id}/send [post]
func sendReportHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			platformhttp.RespondError(c, enum.ErrorCodeUnauthorized, "unauthorized")
			return
		}
		id, ok := parseReportID(c)
		if !ok {
			return
		}
		item, err := svc.MarkSent(c.Request.Context(), id, userID)
		if err != nil {
			respondReportError(c, err)
			return
		}
		platformhttp.RespondOK(c, item)
	}
}

func parseReportID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "invalid report id")
		return 0, false
	}
	return id, true
}

func respondReportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, dto.ReportErrNotFound):
		platformhttp.RespondError(c, enum.ErrorCodeNotFound, "report not found")
	case errors.Is(err, service.ReportErrNoReportSources):
		platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "no report sources")
	case errors.Is(err, service.ReportErrUnsupportedType), errors.Is(err, service.ReportErrInvalidInput):
		platformhttp.RespondError(c, enum.ErrorCodeBadRequest, "")
	default:
		platformhttp.RespondInternalError(c)
	}
}
