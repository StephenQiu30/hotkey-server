package content

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// PostSummary is the query response for content flow endpoints.
type PostSummary struct {
	ID               int64   `json:"id"`
	PlatformPostID   string  `json:"platform_post_id"`
	AuthorName       string  `json:"author_name"`
	AuthorHandle     string  `json:"author_handle"`
	ContentText      string  `json:"content_text"`
	ContentLang      string  `json:"content_lang"`
	PublishedAt      string  `json:"published_at"`
	LikeCount        int     `json:"like_count"`
	ReplyCount       int     `json:"reply_count"`
	RepostCount      int     `json:"repost_count"`
	QuoteCount       int     `json:"quote_count"`
	ViewCount        int     `json:"view_count"`
	HeatScore        float64 `json:"heat_score"`
	RelevanceScore   float64 `json:"relevance_score"`
	FreshnessScore   float64 `json:"freshness_score"`
	FinalScore       float64 `json:"final_score"`
	MatchedKeywords  []string `json:"matched_keywords"`
}

// PostQueryService abstracts the read side for post queries.
type PostQueryService interface {
	ListPostsByMonitor(monitorID int64, limit, offset int) ([]PostSummary, error)
}

// PostHandler provides HTTP handlers for content flow endpoints.
type PostHandler struct {
	svc PostQueryService
}

// NewPostHandler creates a PostHandler.
func NewPostHandler(svc PostQueryService) *PostHandler {
	return &PostHandler{svc: svc}
}

// ServeHTTP dispatches requests to the appropriate handler method.
func (h *PostHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ListByMonitor(w, r)
}

// ListByMonitor handles GET /api/v1/monitors/{id}/posts.
func (h *PostHandler) ListByMonitor(w http.ResponseWriter, r *http.Request) {
	monitorIDStr := extractMonitorIDFromPath(r.URL.Path)
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid monitor id"}`, http.StatusBadRequest)
		return
	}

	limit := parseIntParam(r.URL.Query().Get("limit"), 20)
	offset := parseIntParam(r.URL.Query().Get("offset"), 0)

	posts, err := h.svc.ListPostsByMonitor(monitorID, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if posts == nil {
		posts = []PostSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(posts)
}

// extractMonitorIDFromPath extracts the monitor ID from /api/v1/monitors/{id}/posts.
func extractMonitorIDFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "monitors" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// parseIntParam parses a query parameter with a default value.
func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
