package report

import (
	"errors"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/httputil"
	"net/http"
	"time"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *servicereport.Service
}

func New(service *servicereport.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListReports(c *gin.Context) {
	date := c.Query("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_date", "date must use YYYY-MM-DD")
		return
	}
	reports, err := h.service.ListReportsByDate(c.Request.Context(), date)
	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	items := make([]gin.H, 0, len(reports))
	for _, report := range reports {
		items = append(items, reportResponse(report))
	}
	c.JSON(http.StatusOK, gin.H{"reports": items})
}

func (h *Handler) GetReport(c *gin.Context) {
	reportID := c.Param("reportID")
	report, err := h.service.FindReportByID(c.Request.Context(), reportID)
	if err != nil {
		if errors.Is(err, servicereport.ErrNotFound) {
			httputil.WriteError(c, http.StatusNotFound, "report_not_found", "report not found")
			return
		}
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.JSON(http.StatusOK, reportResponse(report))
}

func reportResponse(report servicereport.DailyReport) gin.H {
	refs := make([]gin.H, 0, len(report.SourceRefs))
	for _, ref := range report.SourceRefs {
		refs = append(refs, gin.H{"sourceId": ref.SourceID, "itemId": ref.ItemID, "title": ref.Title, "url": ref.URL})
	}
	return gin.H{
		"id":              report.ID,
		"date":            report.Date,
		"channelId":       report.ChannelID,
		"userId":          report.UserID,
		"promptVersion":   report.PromptVersion,
		"inputHotspotIds": report.InputHotspotIDs,
		"body":            report.Body,
		"status":          report.Status,
		"lastError":       report.LastError,
		"sourceRefs":      refs,
		"createdAt":       report.CreatedAt,
		"updatedAt":       report.UpdatedAt,
	}
}
