package content

import "context"

// UpsertMonitorHit creates or updates a monitor-post hit relationship.
// This is a convenience wrapper around HitRepository.UpsertHit.
func UpsertMonitorHit(ctx context.Context, repo HitRepository, hit MonitorHit) error {
	return repo.UpsertHit(ctx, hit)
}

// GetMonitorHits retrieves all hits for a given monitor.
func GetMonitorHits(ctx context.Context, repo HitRepository, monitorID int64) ([]MonitorHit, error) {
	return repo.GetHitsByMonitor(ctx, monitorID)
}
