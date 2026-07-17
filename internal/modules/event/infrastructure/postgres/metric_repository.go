package postgres

import (
	"context"
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func (repository *Repository) SaveHeatSnapshot(ctx context.Context, result domain.HeatResult) error {
	if !repository.available() || result.EventID <= 0 || result.WindowEnd.IsZero() {
		return sharedrepository.ErrUnavailable
	}
	_, err := repository.runtime.SQL.ExecContext(ctx, `
INSERT INTO event_metric_snapshots (event_id, captured_at, heat_score, trend_score, source_count, content_count, heat_version, evidence_set_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (event_id, captured_at, heat_version, evidence_set_hash) DO NOTHING`, result.EventID, result.WindowEnd.UTC(), result.HeatScore, result.TrendScore, result.SourceCount, result.ContentCount, result.HeatVersion, result.EvidenceSetHash)
	return sharedrepository.MapError(err)
}

func (repository *Repository) LatestHeatSnapshot(ctx context.Context, eventID int64) (domain.HeatResult, error) {
	if !repository.available() || eventID <= 0 {
		return domain.HeatResult{}, sharedrepository.ErrUnavailable
	}
	var result domain.HeatResult
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT event_id, captured_at, heat_score, trend_score, source_count, content_count, heat_version, evidence_set_hash
FROM event_metric_snapshots WHERE event_id = $1 ORDER BY captured_at DESC, id DESC LIMIT 1`, eventID).
		Scan(&result.EventID, &result.WindowEnd, &result.HeatScore, &result.TrendScore, &result.SourceCount, &result.ContentCount, &result.HeatVersion, &result.EvidenceSetHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.HeatResult{}, sharedrepository.ErrNotFound
		}
		return domain.HeatResult{}, sharedrepository.MapError(err)
	}
	return result, nil
}
