package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

const sourceConnectionColumns = `
id, version, source_type, name, endpoint, auth_type, credential_ref, config,
enabled, health_status, terms_policy_url, created_at, updated_at, deleted_at`

const (
	sourceListDefaultLimit = 50
	sourceListMaximumLimit = 200
	sourceListFingerprint  = "source-connections"
)

// Repository owns only source_connections. The Monitor module supplies its
// own usage reader in Task 4 instead of allowing this adapter to join tables
// it does not own.
type Repository struct{ runtime *database.Runtime }

var _ domain.SourceConnectionRepository = (*Repository)(nil)
var _ domain.MetricSourceContextRepository = (*Repository)(nil)

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
    (source_type, name, endpoint, auth_type, credential_ref, config, enabled, health_status, terms_policy_url, body_storage_default_migrated_at)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, now())
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

func (repository *Repository) ListMetricSourceContexts(ctx context.Context, ids []int64) ([]domain.MetricSourceContext, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	unique := uniqueSourceConnectionIDs(ids)
	if len(unique) == 0 {
		return []domain.MetricSourceContext{}, nil
	}
	rows, err := repository.queryRows(ctx, `
SELECT id, source_type
FROM source_connections
WHERE id = ANY($1::bigint[])
ORDER BY id ASC`, unique)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	contexts := make([]domain.MetricSourceContext, 0, len(unique))
	for rows.Next() {
		var context domain.MetricSourceContext
		if err := rows.Scan(&context.SourceConnectionID, &context.SourceType); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		contexts = append(contexts, context)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	if len(contexts) != len(unique) {
		return nil, sharedrepository.ErrNotFound
	}
	return contexts, nil
}

// List is a source-owned, fixed-shape read: it has no user-controlled sort,
// SQL fragments, or joins. Deleted records remain present because the safe
// DTO explicitly exposes their lifecycle state for shared-team management.
func (repository *Repository) List(ctx context.Context, query domain.SourceConnectionListQuery) ([]domain.SourceConnection, string, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, "", sharedrepository.ErrUnavailable
	}
	limit, cursorID, err := sourceListParameters(query)
	if err != nil {
		return nil, "", err
	}
	rows, err := repository.queryRows(ctx, `
SELECT `+sourceConnectionColumns+`
FROM source_connections
WHERE id > $1
ORDER BY id ASC
LIMIT $2`, cursorID, limit+1)
	if err != nil {
		return nil, "", sharedrepository.MapError(err)
	}
	defer rows.Close()

	connections := make([]domain.SourceConnection, 0, limit+1)
	for rows.Next() {
		record, err := scanSourceConnection(rows)
		if err != nil {
			return nil, "", err
		}
		connection, err := record.sourceConnection()
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
		connections = append(connections, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, "", sharedrepository.MapError(err)
	}
	if len(connections) <= limit {
		return connections, "", nil
	}
	connections = connections[:limit]
	nextCursor, err := pagination.Encode("id", false, sourceListFingerprint, connections[len(connections)-1].ID)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode source cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return connections, nextCursor, nil
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
	if err := repository.queryRow(ctx, query, id).Scan(sourceConnectionScanTargets(&record)...); err != nil {
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

func (repository *Repository) queryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, args...)
}

func (repository *Repository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func uniqueSourceConnectionIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			if _, exists := seen[value]; !exists {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
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

func sourceListParameters(query domain.SourceConnectionListQuery) (int, int64, error) {
	limit := query.Limit
	if limit == 0 {
		limit = sourceListDefaultLimit
	}
	if limit < 1 || limit > sourceListMaximumLimit {
		return 0, 0, fmt.Errorf("%w: source list limit must be from 1 to %d", sharedrepository.ErrInvalidInput, sourceListMaximumLimit)
	}
	cursor, err := pagination.Decode(query.Cursor, "id", false, sourceListFingerprint)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: source list cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return limit, cursor.ID, nil
}

func scanSourceConnection(scanner interface{ Scan(...any) error }) (sourceConnectionRecord, error) {
	var record sourceConnectionRecord
	if err := scanner.Scan(sourceConnectionScanTargets(&record)...); err != nil {
		return sourceConnectionRecord{}, sharedrepository.MapError(err)
	}
	return record, nil
}

func sourceConnectionScanTargets(record *sourceConnectionRecord) []any {
	return []any{
		&record.ID, &record.Version, &record.SourceType, &record.Name, &record.Endpoint, &record.AuthType,
		&record.CredentialRef, &record.Config, &record.Enabled, &record.HealthStatus, &record.TermsPolicyURL,
		&record.CreatedAt, &record.UpdatedAt, &record.DeletedAt,
	}
}
