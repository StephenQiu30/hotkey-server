package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type MetricCapabilityRepository struct{ runtime *database.Runtime }

var _ domain.MetricCapabilityProfileRepository = (*MetricCapabilityRepository)(nil)

func NewMetricCapabilityRepository(runtime *database.Runtime) *MetricCapabilityRepository {
	return &MetricCapabilityRepository{runtime: runtime}
}

func (repository *MetricCapabilityRepository) CreateDraft(ctx context.Context, profile *domain.MetricCapabilityProfile) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if profile == nil {
		return fmt.Errorf("%w: metric capability profile is required", sharedrepository.ErrInvalidInput)
	}
	if err := profile.ValidateDraft(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		return transaction.SQL.QueryRowContext(ctx, `
INSERT INTO metric_capability_profiles (
    source_type, profile_version, supports_views, supports_likes, supports_comments, supports_shares,
    independence_strategy, normalization_window_hours, credibility_weight, max_single_item_contribution
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, version`,
			string(profile.SourceType), profile.ProfileVersion, profile.SupportsViews, profile.SupportsLikes,
			profile.SupportsComments, profile.SupportsShares, string(profile.IndependenceStrategy),
			profile.NormalizationWindowHours, profile.CredibilityWeight, profile.MaxSingleItemContribution,
		).Scan(&profile.ID, &profile.Version)
	})
}

func (repository *MetricCapabilityRepository) FindByID(ctx context.Context, id int64) (*domain.MetricCapabilityProfile, error) {
	return repository.find(ctx, id, false)
}

func (repository *MetricCapabilityRepository) LockByID(ctx context.Context, id int64) (*domain.MetricCapabilityProfile, error) {
	return repository.find(ctx, id, true)
}

func (repository *MetricCapabilityRepository) FindPublished(ctx context.Context, sourceType domain.SourceType) (*domain.MetricCapabilityProfile, error) {
	return repository.findPublished(ctx, sourceType, false)
}

func (repository *MetricCapabilityRepository) LockPublished(ctx context.Context, sourceType domain.SourceType) (*domain.MetricCapabilityProfile, error) {
	return repository.findPublished(ctx, sourceType, true)
}

func (repository *MetricCapabilityRepository) Publish(ctx context.Context, profile *domain.MetricCapabilityProfile) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if profile == nil || profile.ID <= 0 || profile.Version <= 0 {
		return fmt.Errorf("%w: metric capability profile id and version are required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var publishedAt time.Time
		err := transaction.SQL.QueryRowContext(ctx, `
UPDATE metric_capability_profiles
SET status = 'published', published_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 AND status = 'draft'
RETURNING version, published_at`, profile.ID, profile.Version).Scan(&profile.Version, &publishedAt)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		profile.Status, profile.PublishedAt, profile.ArchivedAt = domain.MetricCapabilityPublished, &publishedAt, nil
		return nil
	})
}

func (repository *MetricCapabilityRepository) Archive(ctx context.Context, profile *domain.MetricCapabilityProfile) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if profile == nil || profile.ID <= 0 || profile.Version <= 0 {
		return fmt.Errorf("%w: metric capability profile id and version are required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var archivedAt time.Time
		err := transaction.SQL.QueryRowContext(ctx, `
UPDATE metric_capability_profiles
SET status = 'archived', archived_at = now(), version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 AND status IN ('draft','published')
RETURNING version, archived_at`, profile.ID, profile.Version).Scan(&profile.Version, &archivedAt)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		profile.Status, profile.ArchivedAt = domain.MetricCapabilityArchived, &archivedAt
		return nil
	})
}

func (repository *MetricCapabilityRepository) find(ctx context.Context, id int64, lock bool) (*domain.MetricCapabilityProfile, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: metric capability profile id is required", sharedrepository.ErrInvalidInput)
	}
	query := metricCapabilityProfileSelect + ` WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanMetricCapabilityProfile(repository.queryRow(ctx, query, id))
}

func (repository *MetricCapabilityRepository) findPublished(ctx context.Context, sourceType domain.SourceType, lock bool) (*domain.MetricCapabilityProfile, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if !sourceType.Valid() {
		return nil, fmt.Errorf("%w: source type is required", sharedrepository.ErrInvalidInput)
	}
	query := metricCapabilityProfileSelect + ` WHERE source_type = $1 AND status = 'published'`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanMetricCapabilityProfile(repository.queryRow(ctx, query, string(sourceType)))
}

const metricCapabilityProfileSelect = `
SELECT id, version, source_type, profile_version, supports_views, supports_likes, supports_comments, supports_shares,
       independence_strategy, normalization_window_hours, credibility_weight, max_single_item_contribution,
       status, published_at, archived_at
FROM metric_capability_profiles`

func scanMetricCapabilityProfile(row *sql.Row) (*domain.MetricCapabilityProfile, error) {
	var profile domain.MetricCapabilityProfile
	var sourceType, strategy, status string
	var publishedAt, archivedAt sql.NullTime
	if err := row.Scan(
		&profile.ID, &profile.Version, &sourceType, &profile.ProfileVersion, &profile.SupportsViews, &profile.SupportsLikes,
		&profile.SupportsComments, &profile.SupportsShares, &strategy, &profile.NormalizationWindowHours,
		&profile.CredibilityWeight, &profile.MaxSingleItemContribution, &status, &publishedAt, &archivedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, sharedrepository.ErrNotFound
		}
		return nil, sharedrepository.MapError(err)
	}
	profile.SourceType, profile.IndependenceStrategy, profile.Status = domain.SourceType(sourceType), domain.IndependenceStrategy(strategy), domain.MetricCapabilityStatus(status)
	if publishedAt.Valid {
		value := publishedAt.Time
		profile.PublishedAt = &value
	}
	if archivedAt.Valid {
		value := archivedAt.Time
		profile.ArchivedAt = &value
	}
	return &profile, nil
}

func (repository *MetricCapabilityRepository) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, args...)
}

func (repository *MetricCapabilityRepository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}
