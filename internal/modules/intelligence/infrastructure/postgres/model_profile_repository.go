package postgres

import (
	"context"
	"database/sql"
	"fmt"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

// UpdateProfile changes only operational settings on an existing profile. A
// model, provider, task, credential or vector-dimension change creates a new
// semantic identity and must therefore be represented by a new profile.
func (repository *Repository) UpdateProfile(ctx context.Context, profile intelligencedomain.ModelProfile, expectedVersion int64) (intelligencedomain.ModelProfile, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || profile.ID <= 0 || expectedVersion <= 0 {
		return intelligencedomain.ModelProfile{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if err := profile.Validate(); err != nil {
		return intelligencedomain.ModelProfile{}, err
	}
	var updated intelligencedomain.ModelProfile
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		current, deleted, err := readProfile(ctx, transaction.SQL, profile.ID, true)
		if err != nil {
			return err
		}
		if deleted || current.Version != expectedVersion || !current.SameSemanticIdentity(profile) {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
UPDATE ai_model_profiles
SET name=$1, timeout_seconds=$2, max_attempts=$3, max_cost=$4::numeric,
    daily_budget=$5::numeric, fallback_priority=$6, enabled=$7,
    version=version+1, updated_at=now()
WHERE id=$8 AND version=$9 AND deleted_at IS NULL
RETURNING version`,
			profile.Name, profile.TimeoutSeconds, profile.MaxAttempts, profile.MaxCost, nullableCost(profile.DailyBudget),
			profile.FallbackPriority, profile.Enabled, profile.ID, expectedVersion,
		).Scan(&profile.Version); err != nil {
			if err == sql.ErrNoRows {
				return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
			}
			return fmt.Errorf("update AI profile: %w", err)
		}
		updated = profile
		return nil
	})
	return updated, err
}

// SoftDeleteProfile prevents future selection while retaining completed run and
// vector provenance. Restore requires the same optimistic version discipline.
func (repository *Repository) SoftDeleteProfile(ctx context.Context, id, expectedVersion int64) (intelligencedomain.ModelProfile, error) {
	return repository.setProfileDeleted(ctx, id, expectedVersion, true)
}

func (repository *Repository) RestoreProfile(ctx context.Context, id, expectedVersion int64) (intelligencedomain.ModelProfile, error) {
	return repository.setProfileDeleted(ctx, id, expectedVersion, false)
}

func (repository *Repository) setProfileDeleted(ctx context.Context, id, expectedVersion int64, deleted bool) (intelligencedomain.ModelProfile, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || id <= 0 || expectedVersion <= 0 {
		return intelligencedomain.ModelProfile{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	var profile intelligencedomain.ModelProfile
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		whereDeleted := "IS NOT NULL"
		deletedAt := "NULL"
		if deleted {
			whereDeleted = "IS NULL"
			deletedAt = "now()"
		}
		query := `UPDATE ai_model_profiles SET deleted_at = ` + deletedAt + `, version=version+1, updated_at=now()
WHERE id=$1 AND version=$2 AND deleted_at ` + whereDeleted + ` RETURNING version`
		if err := transaction.SQL.QueryRowContext(ctx, query, id, expectedVersion).Scan(&profile.Version); err != nil {
			if err == sql.ErrNoRows {
				return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
			}
			return fmt.Errorf("change AI profile lifecycle: %w", err)
		}
		profile.ID = id
		loaded, _, err := readProfile(ctx, transaction.SQL, id, true)
		if err != nil {
			return err
		}
		profile = loaded
		return nil
	})
	return profile, err
}

func readProfile(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64, lock bool) (intelligencedomain.ModelProfile, bool, error) {
	query := `SELECT id,version,name,task_type,provider,model_name,model_version,credential_ref,embedding_dimensions,
       timeout_seconds,max_attempts,max_cost::text,daily_budget::text,fallback_priority,enabled,deleted_at IS NOT NULL
FROM ai_model_profiles WHERE id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var profile intelligencedomain.ModelProfile
	var credential sql.NullString
	var dimensions sql.NullInt64
	var dailyBudget sql.NullString
	var deleted bool
	if err := queryer.QueryRowContext(ctx, query, id).Scan(
		&profile.ID, &profile.Version, &profile.Name, &profile.TaskType, &profile.Provider, &profile.ModelName, &profile.ModelVersion,
		&credential, &dimensions, &profile.TimeoutSeconds, &profile.MaxAttempts, &profile.MaxCost, &dailyBudget,
		&profile.FallbackPriority, &profile.Enabled, &deleted,
	); err != nil {
		if err == sql.ErrNoRows {
			return intelligencedomain.ModelProfile{}, false, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
		}
		return intelligencedomain.ModelProfile{}, false, fmt.Errorf("read AI profile: %w", err)
	}
	if credential.Valid {
		profile.CredentialRef = &credential.String
	}
	if dimensions.Valid {
		value := int(dimensions.Int64)
		profile.EmbeddingDimensions = &value
	}
	if dailyBudget.Valid {
		profile.DailyBudget = &dailyBudget.String
	}
	return profile, deleted, nil
}

func nullableCost(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
