package trend

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TrendPoint represents a single data point in a trend series.
type TrendPoint struct {
	Time           time.Time `json:"time"`
	HeatScore      float64   `json:"heat_score"`
	TrendVelocity  float64   `json:"trend_velocity"`
	TrendDirection string    `json:"trend_direction"`
}

// TrendQueryService abstracts the read side for trend queries.
type TrendQueryService interface {
	GetTopicTrends(topicID int64, since time.Time) ([]TrendPoint, error)
	GetMonitorTrends(monitorID int64, since time.Time) ([]TrendPoint, error)
}

// TrendHandler provides HTTP handlers for trend endpoints.
type TrendHandler struct {
	svc TrendQueryService
}

// NewTrendHandler creates a TrendHandler.
func NewTrendHandler(svc TrendQueryService) *TrendHandler {
	return &TrendHandler{svc: svc}
}

// ServeHTTP routes requests to the appropriate handler method based on path.
func (h *TrendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "/topics/") {
		h.GetTopicTrends(w, r)
	} else {
		h.GetMonitorTrends(w, r)
	}
}

// GetTopicTrends handles GET /api/v1/topics/{id}/trends.
func (h *TrendHandler) GetTopicTrends(w http.ResponseWriter, r *http.Request) {
	topicIDStr := extractIDFromPath(r.URL.Path, "topics")
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid topic id"}`, http.StatusBadRequest)
		return
	}

	since := parseSince(r.URL.Query().Get("since"))
	points, err := h.svc.GetTopicTrends(topicID, since)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []TrendPoint{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(points)
}

// GetMonitorTrends handles GET /api/v1/monitors/{id}/trends.
func (h *TrendHandler) GetMonitorTrends(w http.ResponseWriter, r *http.Request) {
	monitorIDStr := extractIDFromPath(r.URL.Path, "monitors")
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid monitor id"}`, http.StatusBadRequest)
		return
	}

	since := parseSince(r.URL.Query().Get("since"))
	points, err := h.svc.GetMonitorTrends(monitorID, since)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []TrendPoint{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(points)
}

// extractIDFromPath extracts the numeric ID after a given segment in the URL path.
func extractIDFromPath(path, segment string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == segment && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// parseSince parses an ISO 8601 timestamp, defaulting to 24h ago.
func parseSince(s string) time.Time {
	if s == "" {
		return time.Now().Add(-24 * time.Hour)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().Add(-24 * time.Hour)
	}
	return t
}
