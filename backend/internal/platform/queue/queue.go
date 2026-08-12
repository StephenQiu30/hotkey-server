// Package queue provides the small PostgreSQL-backed job contract used by
// workers. Most jobs use a shared ID/version envelope; explicitly registered
// kinds may instead use one bounded semantic JSON object.
package queue

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Payload struct {
	EntityID      int64     `json:"entity_id"`
	EntityVersion int64     `json:"entity_version"`
	WindowStart   time.Time `json:"window_start,omitempty"`
	WindowEnd     time.Time `json:"window_end,omitempty"`
	InputHash     string    `json:"input_hash,omitempty"`
	TriggerType   string    `json:"trigger_type,omitempty"`
}

// MarshalJSON keeps the queue envelope minimal. time.Time implements
// json.Marshaler, so the standard encoder does not honor omitempty for its
// zero value; representing optional windows as pointers avoids leaking two
// meaningless timestamps into jobs that only need an entity identity.
func (payload Payload) MarshalJSON() ([]byte, error) {
	type wirePayload struct {
		EntityID      int64      `json:"entity_id"`
		EntityVersion int64      `json:"entity_version"`
		WindowStart   *time.Time `json:"window_start,omitempty"`
		WindowEnd     *time.Time `json:"window_end,omitempty"`
		InputHash     string     `json:"input_hash,omitempty"`
		TriggerType   string     `json:"trigger_type,omitempty"`
	}

	var windowStart, windowEnd *time.Time
	if !payload.WindowStart.IsZero() {
		value := payload.WindowStart.UTC()
		windowStart = &value
	}
	if !payload.WindowEnd.IsZero() {
		value := payload.WindowEnd.UTC()
		windowEnd = &value
	}

	return json.Marshal(wirePayload{
		EntityID:      payload.EntityID,
		EntityVersion: payload.EntityVersion,
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		InputHash:     payload.InputHash,
		TriggerType:   payload.TriggerType,
	})
}

func (payload Payload) Validate() error {
	if payload.EntityID <= 0 || payload.EntityVersion < 1 || len(payload.InputHash) > 128 {
		return fmt.Errorf("invalid job payload")
	}
	if payload.WindowStart.IsZero() != payload.WindowEnd.IsZero() || (!payload.WindowStart.IsZero() && payload.WindowEnd.Before(payload.WindowStart)) {
		return fmt.Errorf("invalid job window")
	}
	if payload.TriggerType != "" && payload.TriggerType != "schedule" && payload.TriggerType != "manual" {
		return fmt.Errorf("invalid job trigger type")
	}
	return nil
}

type Job struct {
	ID        int64
	Kind      string
	UniqueKey string
	Payload   Payload
	// DurableArgs is reserved for job kinds with their own bounded semantic
	// queue DTO. Existing versioned-envelope jobs must continue using Payload.
	DurableArgs json.RawMessage
	ScheduledAt time.Time
	MaxAttempts int
	Priority    int
}

func (job Job) Validate() error {
	if !IsKnownKind(job.Kind) || len(job.Kind) > 64 || len(job.UniqueKey) == 0 || len(job.UniqueKey) > MaxUniqueKeyBytes || job.ScheduledAt.IsZero() || job.MaxAttempts < 1 || job.MaxAttempts > 25 || job.Priority < 1 || job.Priority > 100 {
		return fmt.Errorf("invalid job")
	}
	if kindUsesSemanticDurableArgs(job.Kind) {
		if job.Payload != (Payload{}) || !validSemanticDurableArgs(job.DurableArgs) {
			return fmt.Errorf("invalid semantic durable job args")
		}
	} else {
		if len(job.DurableArgs) != 0 {
			return fmt.Errorf("generic payload job cannot include semantic durable args")
		}
		if err := job.Payload.Validate(); err != nil {
			return err
		}
	}
	encoded, err := job.encodedArgs()
	if err != nil {
		return fmt.Errorf("encode job args: %w", err)
	}
	if len(encoded) > MaxPayloadBytes {
		return fmt.Errorf("job payload exceeds %d bytes", MaxPayloadBytes)
	}
	return nil
}

func kindUsesSemanticDurableArgs(kind string) bool {
	return kind == KindGenerateSourceDocument || kind == KindAnalyzeMonitorIntent || kind == KindEvaluatePublishedDocumentMatches ||
		kind == KindBackfillPublishedMonitorMatches || kind == KindProjectAcceptedDocumentMatch || kind == KindExtractAutomaticClaimEvidence
}

func validSemanticDurableArgs(args json.RawMessage) bool {
	trimmed := bytes.TrimSpace(args)
	return len(trimmed) >= 2 && len(args) <= MaxPayloadBytes && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(trimmed)
}

func (job Job) encodedArgs() ([]byte, error) {
	if kindUsesSemanticDurableArgs(job.Kind) {
		return append([]byte(nil), job.DurableArgs...), nil
	}
	return json.Marshal(job.Payload)
}

type Store struct{ runtime *database.Runtime }

func NewStore(runtime *database.Runtime) *Store { return &Store{runtime: runtime} }

func (store *Store) Enqueue(ctx context.Context, job Job) (int64, bool, error) {
	if store == nil || store.runtime == nil {
		return 0, false, sharedrepository.ErrUnavailable
	}
	if err := job.Validate(); err != nil {
		return 0, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	args, err := job.encodedArgs()
	if err != nil {
		return 0, false, err
	}
	if len(args) > MaxPayloadBytes {
		return 0, false, fmt.Errorf("job payload exceeds %d bytes", MaxPayloadBytes)
	}
	var executor sqlQueryer = store.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		executor = transaction.SQL
	}
	var id int64
	err = executor.QueryRowContext(ctx, `
INSERT INTO river_job (kind, args, state, max_attempts, priority, scheduled_at, unique_key)
VALUES ($1, $2, 'available', $3, $4, $5, $6)
ON CONFLICT (kind, unique_key) DO NOTHING RETURNING id`, job.Kind, args, job.MaxAttempts, job.Priority, job.ScheduledAt.UTC(), []byte(job.UniqueKey)).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, databaserepository.MapError(err)
	}
	err = executor.QueryRowContext(ctx, `SELECT id FROM river_job WHERE kind = $1 AND unique_key = $2`, job.Kind, []byte(job.UniqueKey)).Scan(&id)
	return id, false, databaserepository.MapError(err)
}

// ReactivateByUniqueKey makes an existing collection job immediately
// available without erasing its attempt history. A terminal manual retry
// grants exactly one additional attempt; an already-available automatic retry
// is only brought forward.
func (store *Store) ReactivateByUniqueKey(ctx context.Context, kind, uniqueKey string) (int64, error) {
	if store == nil || store.runtime == nil {
		return 0, sharedrepository.ErrUnavailable
	}
	if !IsKnownKind(kind) || uniqueKey == "" || len(uniqueKey) > MaxUniqueKeyBytes {
		return 0, fmt.Errorf("%w: invalid job identity", sharedrepository.ErrInvalidInput)
	}
	var executor sqlQueryer = store.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		executor = transaction.SQL
	}
	var id int64
	err := executor.QueryRowContext(ctx, `
UPDATE river_job
SET state = 'available',
    max_attempts = CASE WHEN state IN ('discarded', 'cancelled') THEN max_attempts + 1 ELSE max_attempts END,
    scheduled_at = now(),
    finalized_at = NULL
WHERE kind = $1
  AND unique_key = $2
  AND state IN ('available', 'discarded', 'cancelled')
  AND (state = 'available' OR max_attempts < 25)
RETURNING id`, kind, []byte(uniqueKey)).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, databaserepository.MapError(err)
	}
	var state string
	err = executor.QueryRowContext(ctx, `SELECT state FROM river_job WHERE kind = $1 AND unique_key = $2`, kind, []byte(uniqueKey)).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: job not found", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	return 0, fmt.Errorf("%w: job state %s cannot be reactivated", sharedrepository.ErrConflict, state)
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
