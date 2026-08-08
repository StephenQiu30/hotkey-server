package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type RetentionRepository struct{ runtime *database.Runtime }

func NewRetentionRepository(runtime *database.Runtime) *RetentionRepository {
	return &RetentionRepository{runtime: runtime}
}

type retentionQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *RetentionRepository) queryer(ctx context.Context) retentionQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func (repository *RetentionRepository) List(ctx context.Context) ([]operationsdomain.RetentionPolicy, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.queryer(ctx).QueryContext(ctx, `SELECT id,version,data_class,retention_days,action,enabled,description FROM retention_policies ORDER BY id`)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := make([]operationsdomain.RetentionPolicy, 0)
	for rows.Next() {
		var policy operationsdomain.RetentionPolicy
		if err := rows.Scan(&policy.ID, &policy.Version, &policy.DataClass, &policy.RetentionDays, &policy.Action, &policy.Enabled, &policy.Description); err != nil {
			return nil, err
		}
		policy.Protected = policy.DataClass == "audit_logs"
		items = append(items, policy)
	}
	return items, rows.Err()
}

func (repository *RetentionRepository) Find(ctx context.Context, id int64) (operationsdomain.RetentionPolicy, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.RetentionPolicy{}, sharedrepository.ErrUnavailable
	}
	var policy operationsdomain.RetentionPolicy
	err := repository.queryer(ctx).QueryRowContext(ctx, `SELECT id,version,data_class,retention_days,action,enabled,description FROM retention_policies WHERE id=$1`, id).Scan(
		&policy.ID, &policy.Version, &policy.DataClass, &policy.RetentionDays, &policy.Action, &policy.Enabled, &policy.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsdomain.RetentionPolicy{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return operationsdomain.RetentionPolicy{}, databaserepository.MapError(err)
	}
	policy.Protected = policy.DataClass == "audit_logs"
	return policy, nil
}

func (repository *RetentionRepository) Preview(ctx context.Context, policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int) (int64, bool, error) {
	query, args, err := retentionPreviewQuery(policy, cutoff, batchSize)
	if err != nil {
		return 0, false, err
	}
	var candidates int64
	if err := repository.queryer(ctx).QueryRowContext(ctx, query, args...).Scan(&candidates); err != nil {
		return 0, false, databaserepository.MapError(err)
	}
	hasMore := candidates > int64(batchSize)
	if hasMore {
		candidates = int64(batchSize)
	}
	return candidates, hasMore, nil
}

func (repository *RetentionRepository) ApplyRetentionBatch(ctx context.Context, policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int) (int64, error) {
	if repository == nil || repository.runtime == nil {
		return 0, sharedrepository.ErrUnavailable
	}
	query, args, err := retentionDeleteQuery(policy, cutoff, batchSize)
	if err != nil {
		return 0, err
	}
	result, err := repository.queryer(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	return result.RowsAffected()
}

// ApplyRetention preserves the legacy repository port while routing every
// caller through the same bounded implementation.
func (repository *RetentionRepository) ApplyRetention(ctx context.Context, policy operationsdomain.RetentionPolicy, cutoff time.Time) (int64, error) {
	return repository.ApplyRetentionBatch(ctx, policy, cutoff, 1000)
}

func retentionCandidate(policy operationsdomain.RetentionPolicy) (table, predicate, order string, err error) {
	if policy.Protected || policy.DataClass == "audit_logs" {
		return "", "", "", fmt.Errorf("%w: protected retention data class", sharedrepository.ErrInvalidInput)
	}
	switch policy.DataClass {
	case "captured_items":
		return "collection_run_items", "created_at < $1 AND (content_id IS NOT NULL OR outcome <> 'captured')", "id", nil
	case "content_metric_snapshots":
		return "content_metric_snapshots", "captured_at < $1", "id", nil
	case "event_metric_snapshots":
		return "event_metric_snapshots", "captured_at < $1", "id", nil
	case "sessions":
		return "auth_sessions", "absolute_expires_at < $1 OR (revoked_at IS NOT NULL AND revoked_at < $1)", "id", nil
	case "delivery_attempts":
		return "delivery_attempts", "created_at < $1", "id", nil
	case "job_attempts":
		return "river_job_attempt", "created_at < $1", "id", nil
	default:
		return "", "", "", fmt.Errorf("%w: unsupported retention data class %q", sharedrepository.ErrInvalidInput, policy.DataClass)
	}
}

func validateRetentionRun(policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int) error {
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if cutoff.IsZero() || batchSize < 1 || batchSize > 1000 {
		return fmt.Errorf("%w: invalid retention boundary", sharedrepository.ErrInvalidInput)
	}
	if !policy.Enabled {
		return fmt.Errorf("%w: retention policy is disabled", sharedrepository.ErrConflict)
	}
	if policy.Action != "delete" {
		return fmt.Errorf("%w: unsupported retention action", sharedrepository.ErrInvalidInput)
	}
	return nil
}

func retentionPreviewQuery(policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int) (string, []any, error) {
	if err := validateRetentionRun(policy, cutoff, batchSize); err != nil {
		return "", nil, err
	}
	table, predicate, order, err := retentionCandidate(policy)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("SELECT count(*) FROM (SELECT id FROM %s WHERE %s ORDER BY %s LIMIT $2) candidates", table, predicate, order), []any{cutoff, batchSize + 1}, nil
}

func retentionDeleteQuery(policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int) (string, []any, error) {
	if err := validateRetentionRun(policy, cutoff, batchSize); err != nil {
		return "", nil, err
	}
	table, predicate, order, err := retentionCandidate(policy)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("WITH candidates AS (SELECT id FROM %s WHERE %s ORDER BY %s LIMIT $2 FOR UPDATE SKIP LOCKED) DELETE FROM %s target USING candidates WHERE target.id=candidates.id", table, predicate, order, table), []any{cutoff, batchSize}, nil
}
