package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

const sourceConnectionColumns = `
id, version, source_type, name, endpoint, auth_type, credential_ref, config,
enabled, health_status, terms_policy_url, created_at, updated_at, deleted_at`

// Repository owns only source_connections. The Monitor module supplies its
// own usage reader in Task 4 instead of allowing this adapter to join tables
// it does not own.
type Repository struct{ runtime *database.Runtime }

var _ domain.SourceConnectionRepository = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) Create(ctx context.Context, connection *domain.SourceConnection) error {
	if connection == nil {
		return fmt.Errorf("%w: source connection is required", sharedrepository.ErrInvalidInput)
	}
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		config, err := encodeConfig(connection.Config)
		if err != nil {
			return err
		}
		var credentialRef any
		if connection.CredentialRef != "" {
			credentialRef = connection.CredentialRef
		}
		var termsURL any
		if strings.TrimSpace(connection.TermsPolicyURL) != "" {
			termsURL = strings.TrimSpace(connection.TermsPolicyURL)
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO source_connections
    (source_type, name, endpoint, auth_type, credential_ref, config, enabled, health_status, terms_policy_url)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
RETURNING id, version`,
			string(connection.SourceType), connection.Name, connection.Endpoint, string(connection.AuthType), credentialRef,
			config, connection.Enabled, string(connection.HealthStatus), termsURL).Scan(&connection.ID, &connection.Version); err != nil {
			return sharedrepository.MapError(err)
		}
		return nil
	})
}

func (repository *Repository) FindByID(ctx context.Context, id int64) (*domain.SourceConnection, error) {
	return repository.find(ctx, id, false)
}

func (repository *Repository) LockByID(ctx context.Context, id int64) (*domain.SourceConnection, error) {
	return repository.find(ctx, id, true)
}

func (repository *Repository) Update(ctx context.Context, connection *domain.SourceConnection) error {
	if connection == nil || connection.ID <= 0 || connection.Version <= 0 {
		return fmt.Errorf("%w: source connection id and version are required", sharedrepository.ErrInvalidInput)
	}
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		config, err := encodeConfig(connection.Config)
		if err != nil {
			return err
		}
		var credentialRef any
		if connection.CredentialRef != "" {
			credentialRef = connection.CredentialRef
		}
		var termsURL any
		if strings.TrimSpace(connection.TermsPolicyURL) != "" {
			termsURL = strings.TrimSpace(connection.TermsPolicyURL)
		}
		var deletedAt any
		if connection.Deleted {
			deletedAt = sql.NullTime{Time: timeNowUTC(), Valid: true}
		}
		previousVersion := connection.Version
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE source_connections
SET source_type = $1, name = $2, endpoint = $3, auth_type = $4, credential_ref = $5,
    config = $6::jsonb, enabled = $7, health_status = $8, terms_policy_url = $9,
    deleted_at = $10, version = version + 1, updated_at = now()
WHERE id = $11 AND version = $12`,
			string(connection.SourceType), connection.Name, connection.Endpoint, string(connection.AuthType), credentialRef,
			config, connection.Enabled, string(connection.HealthStatus), termsURL, deletedAt, connection.ID, previousVersion)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("%w: source connection was changed or removed", sharedrepository.ErrConflict)
		}
		connection.Version = previousVersion + 1
		return nil
	})
}

func (repository *Repository) HasPublishedReference(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, fmt.Errorf("%w: source connection id is required", sharedrepository.ErrInvalidInput)
	}
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return false, sharedrepository.ErrUnavailable
	}
	var referenced bool
	if err := repository.queryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM monitor_sources AS monitor_source
    JOIN monitor_config_versions AS config_version ON config_version.id = monitor_source.config_version_id
    WHERE monitor_source.source_connection_id = $1
      AND config_version.state IN ('published', 'superseded')
)`, id).Scan(&referenced); err != nil {
		return false, sharedrepository.MapError(err)
	}
	return referenced, nil
}

func (repository *Repository) FindPublicByID(ctx context.Context, id int64) (domain.PublicSourceConnection, error) {
	connection, err := repository.FindByID(ctx, id)
	if err != nil {
		return domain.PublicSourceConnection{}, err
	}
	return sourcePublic(*connection), nil
}

func (repository *Repository) FindManagementByID(ctx context.Context, id int64) (domain.ManagementSourceConnection, error) {
	connection, err := repository.FindByID(ctx, id)
	if err != nil {
		return domain.ManagementSourceConnection{}, err
	}
	return sourceManagement(*connection), nil
}

func (repository *Repository) FindForMonitor(ctx context.Context, id int64) (domain.MonitorSourceConnection, error) {
	connection, err := repository.FindByID(ctx, id)
	if err != nil {
		return domain.MonitorSourceConnection{}, err
	}
	return sourceForMonitor(*connection), nil
}

func (repository *Repository) LockForMonitor(ctx context.Context, id int64) (domain.MonitorSourceConnection, error) {
	connection, err := repository.LockByID(ctx, id)
	if err != nil {
		return domain.MonitorSourceConnection{}, err
	}
	return sourceForMonitor(*connection), nil
}

func (repository *Repository) find(ctx context.Context, id int64, lock bool) (*domain.SourceConnection, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: source connection id is required", sharedrepository.ErrInvalidInput)
	}
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query := "SELECT " + sourceConnectionColumns + " FROM source_connections WHERE id = $1"
	if lock {
		query += " FOR UPDATE"
	}
	var record sourceConnectionRecord
	if err := repository.queryRow(ctx, query, id).Scan(
		&record.ID, &record.Version, &record.SourceType, &record.Name, &record.Endpoint, &record.AuthType,
		&record.CredentialRef, &record.Config, &record.Enabled, &record.HealthStatus, &record.TermsPolicyURL,
		&record.CreatedAt, &record.UpdatedAt, &record.DeletedAt,
	); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	connection, err := record.sourceConnection()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return &connection, nil
}

func (repository *Repository) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, args...)
}

func (repository *Repository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func encodeConfig(config domain.SourceConfig) (string, error) {
	normalized, err := config.Normalize()
	if err != nil {
		return "", fmt.Errorf("%w: source config: %v", sharedrepository.ErrInvalidInput, err)
	}
	// SourceConfig.Map deliberately returns ordinary Go slices. Make the two
	// empty defaults explicit here so PostgreSQL receives the design's stable
	// JSON [] rather than Go's nil-slice JSON null (which Schema rejects).
	encodedConfig := normalized.Map()
	if normalized.AllowedLanguages == nil || len(normalized.AllowedLanguages) == 0 {
		encodedConfig["allowed_languages"] = []string{}
	}
	if normalized.AllowedRegions == nil || len(normalized.AllowedRegions) == 0 {
		encodedConfig["allowed_regions"] = []string{}
	}
	encoded, err := json.Marshal(encodedConfig)
	if err != nil {
		return "", fmt.Errorf("%w: encode source config: %v", sharedrepository.ErrInvalidInput, err)
	}
	return string(encoded), nil
}

// timeNowUTC is a variable solely so deterministic repository tests can
// exercise archival without sharing application clock state.
var timeNowUTC = func() time.Time { return time.Now().UTC() }
