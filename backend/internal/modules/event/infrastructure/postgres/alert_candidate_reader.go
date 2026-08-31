package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// AlertCandidateReader is an Event-owned read adapter. It is the only Alert
// dependency allowed to join EventUpdate with the Event-owned monitor_events
// projection.
type AlertCandidateReader struct{ runtime *database.Runtime }

var _ application.AlertCandidateReader = (*AlertCandidateReader)(nil)

func NewAlertCandidateReader(runtime *database.Runtime) *AlertCandidateReader {
	return &AlertCandidateReader{runtime: runtime}
}

func (reader *AlertCandidateReader) ListAlertCandidates(ctx context.Context, ref application.AlertUpdateRef) ([]application.AlertCandidate, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	var eventID int64
	var kind, title, summary string
	var observedAt time.Time
	var encodedReasons, encodedAfterState []byte
	err := reader.runtime.SQL.QueryRowContext(ctx, `
SELECT event_update.event_id, event_update.kind, event.title_zh, event_update.summary,
       array_to_json(event_update.reason_codes), event_update.after_state, event_update.observed_at
FROM event_updates AS event_update
JOIN events AS event ON event.id = event_update.event_id AND event.deleted_at IS NULL
WHERE event_update.id = $1 AND event_update.version = $2 AND event_update.evidence_set_hash = $3`, ref.ID, ref.Version, ref.EvidenceSetHash).
		Scan(&eventID, &kind, &title, &summary, &encodedReasons, &encodedAfterState, &observedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sharedrepository.ErrNotFound
	}
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	reasonCodes := make([]string, 0)
	if len(encodedReasons) > 0 {
		if err := json.Unmarshal(encodedReasons, &reasonCodes); err != nil {
			return nil, fmt.Errorf("decode event update reason codes: %w", err)
		}
	}
	var afterState struct {
		HeatScore   float64 `json:"heat_score"`
		TrendScore  float64 `json:"trend_score"`
		SourceCount int     `json:"source_count"`
	}
	if err := json.Unmarshal(encodedAfterState, &afterState); err != nil {
		return nil, fmt.Errorf("decode event update after state: %w", err)
	}
	momentumScore := (afterState.TrendScore + 100) / 2
	if momentumScore < 0 {
		momentumScore = 0
	}
	if momentumScore > 100 {
		momentumScore = 100
	}
	breadthScore := float64(afterState.SourceCount * 25)
	if breadthScore > 100 {
		breadthScore = 100
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT monitor_event.monitor_id, monitor_event.final_score
FROM monitor_events AS monitor_event
WHERE monitor_event.event_id = $1 AND monitor_event.status = 'visible'
ORDER BY monitor_event.monitor_id ASC`, eventID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]application.AlertCandidate, 0)
	for rows.Next() {
		candidate := application.AlertCandidate{
			EventID: eventID, UpdateKind: kind, TitleSnapshot: title,
			ReasonSnapshot: summary, ReasonCodes: append([]string(nil), reasonCodes...), TriggeredAt: observedAt.UTC(),
			HeatScore: afterState.HeatScore, MomentumScore: momentumScore, BreadthScore: breadthScore,
		}
		if err := rows.Scan(&candidate.MonitorID, &candidate.FinalScore); err != nil {
			return nil, databaserepository.MapError(err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return candidates, nil
}
