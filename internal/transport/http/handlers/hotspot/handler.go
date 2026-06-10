package hotspot

import (
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/httputil"
	"net/http"
	"strconv"
	"time"

	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
	"github.com/gin-gonic/gin"
)

// Handler handles hotspot HTTP endpoints.
type Handler struct {
	service *servicehotspot.ScoringService
}

// New creates a new hotspot handler.
func New(service *servicehotspot.ScoringService) *Handler {
	return &Handler{service: service}
}

// ListHotspots returns hotspot scores sorted by total_score descending.
// Supports optional query parameters: since, until (ISO 8601 date-time).
func (h *Handler) ListHotspots(c *gin.Context) {
	channel := c.Query("channel")
	sinceStr := c.Query("since")
	untilStr := c.Query("until")
	limit, ok := parseIntParam(c, "limit", 20, 1, 100)
	if !ok {
		return
	}
	offset, ok := parseIntParam(c, "offset", 0, 0, 0)
	if !ok {
		return
	}

	var scores []servicehotspot.HotspotScore
	var err error

	if sinceStr != "" || untilStr != "" {
		since, parseErr := parseTimeParam(sinceStr, time.Time{})
		if parseErr != nil {
			httputil.WriteError(c, http.StatusBadRequest, "invalid_since", "invalid since parameter")
			return
		}
		until, parseErr := parseTimeParam(untilStr, time.Now().UTC())
		if parseErr != nil {
			httputil.WriteError(c, http.StatusBadRequest, "invalid_until", "invalid until parameter")
			return
		}
		if !since.IsZero() && since.After(until) {
			httputil.WriteError(c, http.StatusBadRequest, "invalid_time_window", "since must be before or equal to until")
			return
		}
		scores, err = h.service.ListScoresByWindow(c.Request.Context(), since, until)
	} else {
		scores, err = h.service.ListScores(c.Request.Context())
	}

	if err != nil {
		httputil.WriteError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	scores = filterScoresByChannel(scores, channel)
	scores = paginateScores(scores, limit, offset)

	items := make([]gin.H, 0, len(scores))
	for _, score := range scores {
		items = append(items, scoreResponse(score))
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

func parseTimeParam(value string, defaultVal time.Time) (time.Time, error) {
	if value == "" {
		return defaultVal, nil
	}
	return time.Parse(time.RFC3339, value)
}

func parseIntParam(c *gin.Context, name string, defaultVal int, min int, max int) (int, bool) {
	value := c.Query(name)
	if value == "" {
		return defaultVal, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || (max > 0 && parsed > max) {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_"+name, "invalid "+name+" parameter")
		return 0, false
	}
	return parsed, true
}

// GetHotspot returns a single hotspot detail by cluster ID.
func (h *Handler) GetHotspot(c *gin.Context) {
	clusterID := c.Param("hotspotID")
	if clusterID == "" {
		httputil.WriteError(c, http.StatusBadRequest, "invalid_hotspot_id", "hotspot ID is required")
		return
	}

	score, err := h.service.FindScoreByClusterID(c.Request.Context(), clusterID)
	if err != nil {
		httputil.WriteError(c, http.StatusNotFound, "hotspot_not_found", "hotspot not found")
		return
	}

	c.JSON(http.StatusOK, scoreResponse(score))
}

func filterScoresByChannel(scores []servicehotspot.HotspotScore, channel string) []servicehotspot.HotspotScore {
	if channel == "" {
		return scores
	}
	filtered := make([]servicehotspot.HotspotScore, 0, len(scores))
	for _, score := range scores {
		for _, channelID := range score.ChannelIDs {
			if channelID == channel {
				filtered = append(filtered, score)
				break
			}
		}
	}
	return filtered
}

func paginateScores(scores []servicehotspot.HotspotScore, limit int, offset int) []servicehotspot.HotspotScore {
	if offset >= len(scores) {
		return nil
	}
	end := offset + limit
	if end > len(scores) {
		end = len(scores)
	}
	return scores[offset:end]
}

func scoreResponse(score servicehotspot.HotspotScore) gin.H {
	channelIDs := append([]string{}, score.ChannelIDs...)
	sourceRefs := append([]servicehotspot.SourceRef{}, score.SourceRefs...)
	return gin.H{
		"id":               score.ID,
		"clusterId":        score.ClusterID,
		"totalScore":       score.TotalScore,
		"sourceCountScore": score.SourceCountScore,
		"freshnessScore":   score.FreshnessScore,
		"relevanceScore":   score.RelevanceScore,
		"propagationScore": score.PropagationScore,
		"qualityScore":     score.QualityScore,
		"explanation":      score.Explanation,
		"scoreVersion":     score.ScoreVersion,
		"channelIDs":       channelIDs,
		"sourceRefs":       sourceRefs,
		"createdAt":        score.CreatedAt,
		"updatedAt":        score.UpdatedAt,
	}
}
