package postgres

import (
	"context"
	"fmt"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// RetentionRepository intentionally uses a closed whitelist. Policy data_class
// is operator input and must never become an interpolated SQL identifier.
type RetentionRepository struct{ runtime *database.Runtime }

var _ operationsapplication.RetentionStore = (*RetentionRepository)(nil)

func NewRetentionRepository(runtime *database.Runtime) *RetentionRepository {
	return &RetentionRepository{runtime: runtime}
}

func (repository *RetentionRepository) ApplyRetention(ctx context.Context, policy operationsdomain.RetentionPolicy, cutoff time.Time) (int64, error) {
	if repository == nil || repository.runtime == nil {
		return 0, sharedrepository.ErrUnavailable
	}
	if err := policy.Validate(); err != nil {
		return 0, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if !policy.Enabled {
		return 0, nil
	}
	query, args, err := retentionQuery(policy, cutoff)
	if err != nil {
		return 0, err
	}
	result, err := repository.runtime.SQL.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, sharedrepository.MapError(err)
	}
	return result.RowsAffected()
}

func retentionQuery(policy operationsdomain.RetentionPolicy, cutoff time.Time) (string, []any, error) {
	if cutoff.IsZero() {
		return "", nil, fmt.Errorf("%w: cutoff is required", sharedrepository.ErrInvalidInput)
	}
	// Business tables are archived by soft-delete; append-only operational
	// tables are physically removed only when the policy explicitly says delete.
	if policy.Action == "archive" {
		switch policy.DataClass {
		case "users", "monitors", "source_connections", "contents", "events", "entities", "topics", "knowledge_annotations", "reports", "report_subscriptions", "ai_model_profiles":
			return fmt.Sprintf("UPDATE %s SET deleted_at = now(), updated_at = now() WHERE deleted_at IS NULL AND created_at < $1", policy.DataClass), []any{cutoff}, nil
		default:
			return "", nil, fmt.Errorf("%w: unsupported archive data class %q", sharedrepository.ErrInvalidInput, policy.DataClass)
		}
	}
	switch policy.DataClass {
	case "delivery_attempts":
		return "DELETE FROM delivery_attempts WHERE created_at < $1", []any{cutoff}, nil
	case "audit_logs":
		return "DELETE FROM audit_logs WHERE created_at < $1", []any{cutoff}, nil
	case "event_metric_snapshots":
		return "DELETE FROM event_metric_snapshots WHERE captured_at < $1", []any{cutoff}, nil
	case "river_job_attempt":
		return "DELETE FROM river_job_attempt WHERE created_at < $1", []any{cutoff}, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported delete data class %q", sharedrepository.ErrInvalidInput, policy.DataClass)
	}
}
