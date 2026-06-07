package eventsummary

import (
	"errors"
	"net/http"

	serviceeventsummary "github.com/StephenQiu30/hotkey-server/internal/service/eventsummary"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *serviceeventsummary.Service
}

func New(service *serviceeventsummary.Service) *Handler {
	return &Handler{service: service}
}

// GetSummary returns the summary for a given event ID.
func (h *Handler) GetSummary(c *gin.Context) {
	eventID := c.Param("eventID")
	if eventID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "eventID is required")
		return
	}
	summary, err := h.service.GetSummary(c.Request.Context(), eventID)
	if err != nil {
		if errors.Is(err, serviceeventsummary.ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if errors.Is(err, serviceeventsummary.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "event summary not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.JSON(http.StatusOK, summaryResponse(summary))
}

// GenerateSummaryRequest is the request body for GenerateSummary.
type GenerateSummaryRequest struct {
	EventID string                       `json:"eventId"`
	Title   string                       `json:"title"`
	Items   []serviceeventsummary.ItemInfo `json:"items"`
}

// GenerateSummary triggers summary generation for an event.
func (h *Handler) GenerateSummary(c *gin.Context) {
	var req GenerateSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.EventID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "eventId is required")
		return
	}
	if req.Title == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	input := serviceeventsummary.GenerateSummaryInput{
		EventID: req.EventID,
		Title:   req.Title,
		Items:   req.Items,
	}

	summary, err := h.service.GenerateSummary(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, serviceeventsummary.ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	c.JSON(http.StatusOK, summaryResponse(summary))
}

func summaryResponse(s serviceeventsummary.EventSummary) gin.H {
	timeline := make([]gin.H, 0, len(s.Timeline))
	for _, t := range s.Timeline {
		timeline = append(timeline, gin.H{"date": t.Date, "description": t.Description})
	}
	refs := make([]gin.H, 0, len(s.SourceRefs))
	for _, r := range s.SourceRefs {
		refs = append(refs, gin.H{"sourceId": r.SourceID, "itemId": r.ItemID, "title": r.Title, "url": r.URL})
	}
	return gin.H{
		"id":            s.ID,
		"eventId":       s.EventID,
		"promptVersion": s.PromptVersion,
		"title":         s.Title,
		"summary":       s.Summary,
		"timeline":      timeline,
		"keySignals":    s.KeySignals,
		"sourceRefs":    refs,
		"riskAlerts":    s.RiskAlerts,
		"followUp":      s.FollowUp,
		"confidence":    s.Confidence,
		"modelStatus":   s.ModelStatus,
		"lastError":     s.LastError,
		"version":       s.Version,
		"lowEvidence":   s.LowEvidence,
		"createdAt":     s.CreatedAt,
		"updatedAt":     s.UpdatedAt,
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
