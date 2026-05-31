package worker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
)

// HotspotScoringService is the interface needed by the scoring worker handler.
type HotspotScoringService interface {
	ScoreClusters(context.Context, time.Time, time.Time) ([]servicehotspot.HotspotScore, error)
}

// ScoreHotspotsHandler handles score_hotspots jobs.
type ScoreHotspotsHandler struct {
	service HotspotScoringService
	now     func() time.Time
}

// NewScoreHotspotsHandler creates a new score hotspots handler.
func NewScoreHotspotsHandler(service HotspotScoringService) *ScoreHotspotsHandler {
	return &ScoreHotspotsHandler{
		service: service,
		now:     time.Now,
	}
}

func (h *ScoreHotspotsHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload queue.ScoreHotspotsPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload.ClusterRunID == "" {
		return errors.New("score_hotspots payload missing cluster_run_id")
	}

	now := h.now().UTC()
	windowStart := now.Add(-24 * time.Hour)
	_, err := h.service.ScoreClusters(ctx, windowStart, now)
	return err
}
