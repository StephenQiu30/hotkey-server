package topic

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// TopicQueryService abstracts the read side for topic queries.
type TopicQueryService interface {
	ListByMonitor(monitorID int64) ([]TopicSummary, error)
}

// TopicHandler provides HTTP handlers for topic endpoints.
type TopicHandler struct {
	svc TopicQueryService
}

// NewTopicHandler creates a TopicHandler.
func NewTopicHandler(svc TopicQueryService) *TopicHandler {
	return &TopicHandler{svc: svc}
}

// ListByMonitor handles GET /api/v1/monitors/{id}/topics.
func (h *TopicHandler) ListByMonitor(w http.ResponseWriter, r *http.Request) {
	monitorIDStr := extractIDFromPath(r.URL.Path, "monitors")
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid monitor id"}`, http.StatusBadRequest)
		return
	}

	topics, err := h.svc.ListByMonitor(monitorID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if topics == nil {
		topics = []TopicSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(topics)
}

// extractIDFromPath extracts the numeric ID after a given segment in the URL path.
// e.g. "/api/v1/monitors/123/topics" with segment "monitors" => "123"
func extractIDFromPath(path, segment string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == segment && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
