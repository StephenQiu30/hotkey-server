package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type UpdateRepository struct{ runtime *database.Runtime }

var _ application.UpdateRepository = (*UpdateRepository)(nil)

func NewUpdateRepository(runtime *database.Runtime) *UpdateRepository {
	return &UpdateRepository{runtime: runtime}
}

func (repository *UpdateRepository) PreviousHeatSnapshot(ctx context.Context, eventID int64, windowHours int, before time.Time) (*domain.HeatResult, error) {
	if !repository.available() {
		return nil, sharedrepository.ErrUnavailable
	}
	if eventID <= 0 || windowHours != 24 || before.IsZero() {
		return nil, sharedrepository.ErrInvalidInput
	}
	var query rowQuery = repository.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		query = transaction.SQL
	}
	var result domain.HeatResult
	err := query.QueryRowContext(ctx, `
SELECT event_id, captured_at, window_hours, heat_score, trend_score, trend_status,
       source_count, content_count, heat_version, evidence_set_hash, capability_profile_set_hash
FROM event_metric_snapshots
WHERE event_id = $1 AND window_hours = $2 AND captured_at < $3
ORDER BY captured_at DESC, id DESC
LIMIT 1`, eventID, windowHours, before.UTC()).Scan(
		&result.EventID, &result.WindowEnd, &result.WindowHours, &result.HeatScore, &result.TrendScore, &result.TrendStatus,
		&result.SourceCount, &result.ContentCount, &result.HeatVersion, &result.EvidenceSetHash, &result.CapabilityProfileSetHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	return &result, nil
}

func (repository *UpdateRepository) AppendUpdate(ctx context.Context, candidate domain.EventUpdateCandidate) (*domain.EventUpdate, bool, error) {
	if !repository.available() {
		return nil, false, sharedrepository.ErrUnavailable
	}
	if err := candidate.Validate(); err != nil {
		return nil, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	reasons, err := json.Marshal(candidate.ReasonCodes)
	if err != nil {
		return nil, false, fmt.Errorf("encode event update reasons: %w", err)
	}
	before := []byte("{}")
	if candidate.BeforeState != nil {
		before, err = marshalEventUpdateState(*candidate.BeforeState)
		if err != nil {
			return nil, false, fmt.Errorf("encode event update before state: %w", err)
		}
	}
	after, err := marshalEventUpdateState(candidate.AfterState)
	if err != nil {
		return nil, false, fmt.Errorf("encode event update after state: %w", err)
	}

	var update *domain.EventUpdate
	created := false
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var lockedID int64
		if err := transaction.SQL.QueryRowContext(ctx, `SELECT id FROM events WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, candidate.EventID).Scan(&lockedID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sharedrepository.ErrNotFound
			}
			return databaserepository.MapError(err)
		}
		existing, err := findEventUpdateByKey(ctx, transaction.SQL, candidate.EventID, candidate.IdempotencyKey)
		if err == nil {
			update = existing
			return nil
		}
		if !errors.Is(err, sharedrepository.ErrNotFound) {
			return err
		}

		var sequence int64
		if err := transaction.SQL.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_no), 0) + 1 FROM event_updates WHERE event_id = $1`, candidate.EventID).Scan(&sequence); err != nil {
			return databaserepository.MapError(err)
		}
		row := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO event_updates (
    event_id, sequence_no, kind, summary, observed_at, reason_codes,
    before_state, after_state, evidence_set_hash, idempotency_key
)
VALUES ($1,$2,$3,$4,$5,ARRAY(SELECT jsonb_array_elements_text($6::jsonb)),$7::jsonb,$8::jsonb,$9,$10)
ON CONFLICT (event_id, idempotency_key) DO NOTHING
RETURNING `+eventUpdateColumns,
			candidate.EventID, sequence, string(candidate.Kind), candidate.Summary, candidate.ObservedAt.UTC(), string(reasons),
			string(before), string(after), candidate.EvidenceSetHash, candidate.IdempotencyKey,
		)
		inserted, scanErr := scanEventUpdate(row)
		if scanErr == nil {
			update, created = inserted, true
			return nil
		}
		if !errors.Is(scanErr, sharedrepository.ErrNotFound) {
			return scanErr
		}
		existing, err = findEventUpdateByKey(ctx, transaction.SQL, candidate.EventID, candidate.IdempotencyKey)
		if err != nil {
			return err
		}
		update = existing
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return update, created, nil
}

func (repository *UpdateRepository) ListUpdates(ctx context.Context, query domain.EventUpdateListQuery) (domain.EventUpdatePage, error) {
	if !repository.available() {
		return domain.EventUpdatePage{}, sharedrepository.ErrUnavailable
	}
	if query.EventID <= 0 || query.Limit < 1 || query.Limit > 100 || query.Cursor < 0 {
		return domain.EventUpdatePage{}, sharedrepository.ErrInvalidInput
	}
	var executor metricQuery = repository.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		executor = transaction.SQL
	}
	var existingEventID int64
	if err := executor.QueryRowContext(ctx, `SELECT id FROM events WHERE id = $1 AND deleted_at IS NULL`, query.EventID).Scan(&existingEventID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EventUpdatePage{}, sharedrepository.ErrNotFound
		}
		return domain.EventUpdatePage{}, databaserepository.MapError(err)
	}
	rows, err := executor.QueryContext(ctx, `
SELECT `+eventUpdateColumns+`
FROM event_updates
WHERE event_id = $1 AND ($2::bigint = 0 OR sequence_no < $2)
ORDER BY sequence_no DESC, id DESC
LIMIT $3`, query.EventID, query.Cursor, query.Limit+1)
	if err != nil {
		return domain.EventUpdatePage{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.EventUpdate, 0, query.Limit+1)
	for rows.Next() {
		update, err := scanEventUpdate(rows)
		if err != nil {
			return domain.EventUpdatePage{}, err
		}
		items = append(items, *update)
	}
	if err := rows.Err(); err != nil {
		return domain.EventUpdatePage{}, databaserepository.MapError(err)
	}
	page := domain.EventUpdatePage{Items: items}
	if len(items) > query.Limit {
		page.NextCursor = items[query.Limit-1].SequenceNo
		page.Items = items[:query.Limit]
	}
	return page, nil
}

const eventUpdateColumns = `id, version, event_id, sequence_no, kind, summary, observed_at,
       array_to_json(reason_codes), before_state, after_state, evidence_set_hash, idempotency_key, created_at`

func findEventUpdateByKey(ctx context.Context, query rowQuery, eventID int64, key string) (*domain.EventUpdate, error) {
	return scanEventUpdate(query.QueryRowContext(ctx, `SELECT `+eventUpdateColumns+` FROM event_updates WHERE event_id = $1 AND idempotency_key = $2`, eventID, key))
}

func scanEventUpdate(scanner rowScanner) (*domain.EventUpdate, error) {
	var update domain.EventUpdate
	var reasons, before, after []byte
	if err := scanner.Scan(
		&update.ID, &update.Version, &update.EventID, &update.SequenceNo, &update.Kind, &update.Summary, &update.ObservedAt,
		&reasons, &before, &after, &update.EvidenceSetHash, &update.IdempotencyKey, &update.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sharedrepository.ErrNotFound
		}
		return nil, databaserepository.MapError(err)
	}
	if err := json.Unmarshal(reasons, &update.ReasonCodes); err != nil {
		return nil, fmt.Errorf("decode event update reasons: %w", err)
	}
	if !bytes.Equal(bytes.TrimSpace(before), []byte("{}")) {
		state, err := unmarshalEventUpdateState(before, update.EventID)
		if err != nil {
			return nil, fmt.Errorf("decode event update before state: %w", err)
		}
		update.BeforeState = state
	}
	afterState, err := unmarshalEventUpdateState(after, update.EventID)
	if err != nil {
		return nil, fmt.Errorf("decode event update after state: %w", err)
	}
	update.AfterState = *afterState
	if update.ReasonCodes == nil {
		update.ReasonCodes = []string{}
	}
	return &update, nil
}

type persistedEventUpdateState struct {
	HeatScore                float64            `json:"heat_score"`
	TrendScore               float64            `json:"trend_score"`
	TrendStatus              domain.TrendStatus `json:"trend_status"`
	SourceCount              int                `json:"source_count"`
	ContentCount             int                `json:"content_count"`
	WindowEnd                time.Time          `json:"window_end"`
	WindowHours              int                `json:"window_hours"`
	HeatVersion              string             `json:"heat_version"`
	EvidenceSetHash          string             `json:"evidence_set_hash"`
	CapabilityProfileSetHash string             `json:"capability_profile_set_hash"`
}

func marshalEventUpdateState(state domain.HeatResult) ([]byte, error) {
	return json.Marshal(persistedEventUpdateState{
		HeatScore: state.HeatScore, TrendScore: state.TrendScore, TrendStatus: state.TrendStatus,
		SourceCount: state.SourceCount, ContentCount: state.ContentCount, WindowEnd: state.WindowEnd.UTC(), WindowHours: state.WindowHours,
		HeatVersion: state.HeatVersion, EvidenceSetHash: state.EvidenceSetHash, CapabilityProfileSetHash: state.CapabilityProfileSetHash,
	})
}

func unmarshalEventUpdateState(encoded []byte, eventID int64) (*domain.HeatResult, error) {
	var stored persistedEventUpdateState
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, err
	}
	return &domain.HeatResult{
		EventID: eventID, HeatScore: stored.HeatScore, TrendScore: stored.TrendScore, TrendStatus: stored.TrendStatus,
		SourceCount: stored.SourceCount, ContentCount: stored.ContentCount, WindowEnd: stored.WindowEnd.UTC(), WindowHours: stored.WindowHours,
		HeatVersion: stored.HeatVersion, EvidenceSetHash: stored.EvidenceSetHash, CapabilityProfileSetHash: stored.CapabilityProfileSetHash,
	}, nil
}

func (repository *UpdateRepository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func (repository *UpdateRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}
