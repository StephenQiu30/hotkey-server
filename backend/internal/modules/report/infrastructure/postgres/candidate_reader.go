package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	reportapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// CandidateReader freezes the latest EventUpdate observed inside the report
// period. The mutable Event row contributes only its display title.
type CandidateReader struct{ runtime *database.Runtime }

func NewCandidateReader(runtime *database.Runtime) *CandidateReader {
	return &CandidateReader{runtime: runtime}
}

func (reader *CandidateReader) ListForPeriod(ctx context.Context, monitorID *int64, start, end time.Time, limit int) ([]reportapplication.EventSnapshot, error) {
	if reader == nil || reader.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if start.IsZero() || !end.After(start) || limit < 1 || limit > 100 {
		return nil, sharedrepository.ErrInvalidInput
	}
	rows, err := reportQueryerFor(ctx, reader.runtime).QueryContext(ctx, `
SELECT event_row.id, update_row.id, event_row.title_zh, update_row.summary,
       COALESCE((update_row.after_state->>'heat_score')::double precision, 0),
       update_row.evidence_set_hash, array_to_json(update_row.reason_codes)
FROM events event_row
JOIN LATERAL (
    SELECT id, summary, after_state, evidence_set_hash, reason_codes
    FROM event_updates
    WHERE event_id = event_row.id AND observed_at >= $1 AND observed_at < $2
    ORDER BY observed_at DESC, id DESC
    LIMIT 1
) update_row ON true
WHERE event_row.deleted_at IS NULL
  AND event_row.lifecycle_status NOT IN ('rejected','merged','archived')
  AND ($3::bigint IS NULL OR EXISTS (
      SELECT 1 FROM monitor_events scope
      WHERE scope.event_id = event_row.id AND scope.monitor_id = $3 AND scope.status <> 'hidden'
  ))
ORDER BY COALESCE((update_row.after_state->>'heat_score')::double precision, 0) DESC, event_row.id ASC
LIMIT $4`, start.UTC(), end.UTC(), monitorID, limit)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := make([]reportapplication.EventSnapshot, 0)
	for rows.Next() {
		var item reportapplication.EventSnapshot
		var reasons []byte
		if err := rows.Scan(&item.EventID, &item.EventUpdateID, &item.Title, &item.Summary, &item.HeatScore, &item.EvidenceSetHash, &reasons); err != nil {
			return nil, databaserepository.MapError(err)
		}
		if err := json.Unmarshal(reasons, &item.ReasonCodes); err != nil {
			return nil, fmt.Errorf("decode report candidate reasons: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}

var _ reportapplication.SnapshotReader = (*CandidateReader)(nil)
