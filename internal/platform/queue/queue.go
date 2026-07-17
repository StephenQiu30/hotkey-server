// Package queue provides the small PostgreSQL-backed job contract used by
// workers. The payload is intentionally an ID/version envelope rather than
// arbitrary business JSON.
package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type Payload struct {
	EntityID      int64     `json:"entity_id"`
	EntityVersion int64     `json:"entity_version"`
	WindowStart   time.Time `json:"window_start,omitempty"`
	WindowEnd     time.Time `json:"window_end,omitempty"`
	InputHash     string    `json:"input_hash,omitempty"`
}

func (payload Payload) Validate() error {
	if payload.EntityID <= 0 || payload.EntityVersion < 0 || len(payload.InputHash) > 128 {
		return fmt.Errorf("invalid job payload")
	}
	return nil
}

type Job struct {
	ID          int64
	Kind        string
	UniqueKey   string
	Payload     Payload
	ScheduledAt time.Time
	MaxAttempts int
	Priority    int
}

func (job Job) Validate() error {
	if job.Kind == "" || job.UniqueKey == "" || job.ScheduledAt.IsZero() || job.MaxAttempts < 1 || job.MaxAttempts > 25 || job.Priority < 1 || job.Priority > 100 {
		return fmt.Errorf("invalid job")
	}
	return job.Payload.Validate()
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
	args, err := json.Marshal(job.Payload)
	if err != nil {
		return 0, false, err
	}
	var id int64
	err = store.runtime.SQL.QueryRowContext(ctx, `
INSERT INTO river_job (kind, args, state, max_attempts, priority, scheduled_at, unique_key)
VALUES ($1, $2, 'available', $3, $4, $5, $6)
ON CONFLICT (kind, unique_key) DO NOTHING RETURNING id`, job.Kind, args, job.MaxAttempts, job.Priority, job.ScheduledAt.UTC(), []byte(job.UniqueKey)).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, sharedrepository.MapError(err)
	}
	err = store.runtime.SQL.QueryRowContext(ctx, `SELECT id FROM river_job WHERE kind = $1 AND unique_key = $2`, job.Kind, []byte(job.UniqueKey)).Scan(&id)
	return id, false, sharedrepository.MapError(err)
}
