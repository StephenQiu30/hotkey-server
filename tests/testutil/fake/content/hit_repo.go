package fakecontent

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// HitRepo is an in-memory fake implementing content.HitRepository.
type HitRepo struct {
	Hits []content.MonitorHit
}

func (r *HitRepo) UpsertHit(_ context.Context, hit content.MonitorHit) error {
	for i := range r.Hits {
		if r.Hits[i].MonitorID == hit.MonitorID && r.Hits[i].PostID == hit.PostID {
			r.Hits[i] = hit
			return nil
		}
	}
	r.Hits = append(r.Hits, hit)
	return nil
}

func (r *HitRepo) GetHitsByMonitor(_ context.Context, monitorID int64) ([]content.MonitorHit, error) {
	var out []content.MonitorHit
	for _, h := range r.Hits {
		if h.MonitorID == monitorID {
			out = append(out, h)
		}
	}
	return out, nil
}
