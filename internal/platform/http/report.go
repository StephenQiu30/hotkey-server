package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/report"
)

type ReportService interface {
	Create(ctx context.Context, userID int64, input dto.CreateInput) (dto.Report, error)
	List(ctx context.Context, userID int64, filter dto.ListFilter) ([]dto.Report, int64, error)
	GetByID(ctx context.Context, id, userID int64) (dto.Report, error)
	HTML(ctx context.Context, id, userID int64) (string, error)
	MarkSent(ctx context.Context, id, userID int64) (dto.Report, error)
}

type CreateReportRequest struct {
	ReportType  string `json:"report_type" example:"weekly"`
	PeriodStart string `json:"period_start,omitempty" example:"2026-06-24"`
	PeriodEnd   string `json:"period_end,omitempty" example:"2026-06-30"`
	Send        bool   `json:"send" example:"false"`
}

func RegisterReportRoutes(r *gin.Engine, svc ReportService) {
	if svc == nil {
		return
	}
	r.GET("/api/v1/reports", listReportsHandler(svc))
	r.POST("/api/v1/reports", createReportHandler(svc))
	r.GET("/api/v1/reports/:id", getReportHandler(svc))
	r.GET("/api/v1/reports/:id/html", getReportHTMLHandler(svc))
	r.POST("/api/v1/reports/:id/send", sendReportHandler(svc))
}

func createReportHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req CreateReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "invalid input")
			return
		}

		input, err := req.toInput()
		if err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}

		item, err := svc.Create(c.Request.Context(), userID, input)
		if err != nil {
			respondReportError(c, err)
			return
		}
		RespondCreated(c, item)
	}
}

func listReportsHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
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
			respondInternalError(c)
			return
		}
		RespondPage(c, items, offset/limit+1, limit, int(total))
	}
}

func getReportHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
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
		RespondOK(c, item)
	}
}

func getReportHTMLHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
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

func sendReportHandler(svc ReportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := userIDFromCtx(c.Request.Context())
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized")
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
		RespondOK(c, item)
	}
}

func (r CreateReportRequest) toInput() (dto.CreateInput, error) {
	var start *time.Time
	if r.PeriodStart != "" {
		parsed, err := time.Parse("2006-01-02", r.PeriodStart)
		if err != nil {
			return dto.CreateInput{}, errors.New("invalid period_start")
		}
		start = &parsed
	}
	var end *time.Time
	if r.PeriodEnd != "" {
		parsed, err := time.Parse("2006-01-02", r.PeriodEnd)
		if err != nil {
			return dto.CreateInput{}, errors.New("invalid period_end")
		}
		end = &parsed
	}
	return dto.CreateInput{
		ReportType:  r.ReportType,
		PeriodStart: start,
		PeriodEnd:   end,
		Send:        r.Send,
	}, nil
}

func parseReportID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid report id")
		return 0, false
	}
	return id, true
}

func respondReportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, report.ErrNotFound):
		respondError(c, http.StatusNotFound, "report not found")
	case errors.Is(err, report.ErrNoReportSources):
		respondError(c, http.StatusBadRequest, "no report sources")
	case errors.Is(err, report.ErrUnsupportedType), errors.Is(err, report.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, err.Error())
	default:
		respondInternalError(c)
	}
}
