package hotspot

import (
	"net/http"
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
	sinceStr := c.Query("since")
	untilStr := c.Query("until")

	var scores []servicehotspot.HotspotScore
	var err error

	if sinceStr != "" || untilStr != "" {
		since, parseErr := parseTimeParam(sinceStr, time.Time{})
		if parseErr != nil {
			writeError(c, http.StatusBadRequest, "invalid_since", "invalid since parameter")
			return
		}
		until, parseErr := parseTimeParam(untilStr, time.Now().UTC())
		if parseErr != nil {
			writeError(c, http.StatusBadRequest, "invalid_until", "invalid until parameter")
			return
		}
		scores, err = h.service.ListScoresByWindow(c.Request.Context(), since, until)
	} else {
		scores, err = h.service.ListScores(c.Request.Context())
	}

	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

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

// GetHotspot returns a single hotspot detail by cluster ID.
func (h *Handler) GetHotspot(c *gin.Context) {
	clusterID := c.Param("hotspotID")
	if clusterID == "" {
		writeError(c, http.StatusBadRequest, "invalid_hotspot_id", "hotspot ID is required")
		return
	}

	score, err := h.service.FindScoreByClusterID(c.Request.Context(), clusterID)
	if err != nil {
		writeError(c, http.StatusNotFound, "hotspot_not_found", "hotspot not found")
		return
	}

	c.JSON(http.StatusOK, scoreResponse(score))
}

func scoreResponse(score servicehotspot.HotspotScore) gin.H {
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
		"createdAt":        score.CreatedAt,
		"updatedAt":        score.UpdatedAt,
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
