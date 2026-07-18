package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

const monitorColumns = `id, version, name, description, status, draft_config_version_id, published_config_version_id, created_at, updated_at, deleted_at`
const configColumns = `id, version, monitor_id, revision, state, timezone, array_to_json(languages), array_to_json(regions), collection_interval_seconds, relevance_threshold, event_threshold, retention_days, coalesce(config_hash, ''), published_at, created_at, updated_at`
const ruleColumns = `id, version, config_version_id, rule_type, operator, value, weight, priority, origin, approval_status, enabled`
const monitorSourceColumns = `id, version, config_version_id, source_connection_id, query_override, query_signature, priority, enabled`

const (
	monitorListDefaultLimit = 50
	monitorListMaximumLimit = 200
	monitorListFingerprint  = "monitors"
)

type Repository struct{ runtime *database.Runtime }

var _ domain.MonitorRepository = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) Create(ctx context.Context, monitor *domain.Monitor, config *domain.MonitorConfigVersion, rules []domain.MonitorRule, sources []domain.MonitorSource) error {
	if monitor == nil || config == nil {
		return fmt.Errorf("%w: monitor and config are required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := transaction.SQL.QueryRowContext(ctx, `INSERT INTO monitors (name, description, status) VALUES ($1, $2, 'draft') RETURNING `+monitorColumns, monitor.Name, monitor.Description).Scan(monitorScanTargets(monitor)...); err != nil {
			return sharedrepository.MapError(err)
		}
		config.MonitorID = monitor.ID
		config.State = domain.ConfigVersionDraft
		config.Revision = 1
		if err := repository.insertConfig(ctx, transaction.SQL, config); err != nil {
			return err
		}
		if err := repository.insertRules(ctx, transaction.SQL, config.ID, rules); err != nil {
			return err
		}
		if err := repository.insertSources(ctx, transaction.SQL, config.ID, sources); err != nil {
			return err
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE monitors SET draft_config_version_id = $1 WHERE id = $2`, config.ID, monitor.ID); err != nil {
			return sharedrepository.MapError(err)
		}
		monitor.DraftConfigVersionID = int64Pointer(config.ID)
		return nil
	})
}

func (repository *Repository) FindByID(ctx context.Context, id int64) (*domain.Monitor, error) {
	return repository.findMonitor(ctx, id, false)
}
func (repository *Repository) LockByID(ctx context.Context, id int64) (*domain.Monitor, error) {
	return repository.findMonitor(ctx, id, true)
}

// List has a deliberately fixed id-ascending shape. The application selects
// PublishedOnly for viewer reads; editors/admins receive all Monitor metadata
// and decide which safe configuration projection to expose.
func (repository *Repository) List(ctx context.Context, query domain.MonitorListQuery) ([]domain.Monitor, string, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, "", sharedrepository.ErrUnavailable
	}
	limit, cursorID, err := monitorListParameters(query)
	if err != nil {
		return nil, "", err
	}
	statement := `SELECT ` + monitorColumns + ` FROM monitors WHERE id > $1 AND deleted_at IS NULL`
	if query.PublishedOnly {
		statement += ` AND status IN ('active', 'paused') AND published_config_version_id IS NOT NULL`
	}
	statement += ` ORDER BY id ASC LIMIT $2`
	rows, err := repository.runtime.SQL.QueryContext(ctx, statement, cursorID, limit+1)
	if err != nil {
		return nil, "", sharedrepository.MapError(err)
	}
	defer rows.Close()
	monitors := make([]domain.Monitor, 0, limit+1)
	for rows.Next() {
		var monitor domain.Monitor
		if err := rows.Scan(monitorScanTargets(&monitor)...); err != nil {
			return nil, "", sharedrepository.MapError(err)
		}
		monitors = append(monitors, monitor)
	}
	if err := rows.Err(); err != nil {
		return nil, "", sharedrepository.MapError(err)
	}
	if len(monitors) <= limit {
		return monitors, "", nil
	}
	monitors = monitors[:limit]
	fingerprint := monitorListFingerprint
	if query.PublishedOnly {
		fingerprint += "-published"
	}
	nextCursor, err := pagination.Encode("id", false, fingerprint, monitors[len(monitors)-1].ID)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode monitor cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return monitors, nextCursor, nil
}

func (repository *Repository) findMonitor(ctx context.Context, id int64, lock bool) (*domain.Monitor, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: monitor id is required", sharedrepository.ErrInvalidInput)
	}
	var monitor domain.Monitor
	query := `SELECT ` + monitorColumns + ` FROM monitors WHERE id = $1 AND deleted_at IS NULL`
	if lock {
		query += ` FOR UPDATE`
	}
	if err := repository.queryRow(ctx, query, id).Scan(monitorScanTargets(&monitor)...); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return &monitor, nil
}

func (repository *Repository) FindConfig(ctx context.Context, id int64) (*domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	return repository.config(ctx, id, false)
}
func (repository *Repository) LockConfig(ctx context.Context, id int64) (*domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	return repository.config(ctx, id, true)
}

func (repository *Repository) config(ctx context.Context, id int64, lock bool) (*domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	if id <= 0 {
		return nil, nil, nil, fmt.Errorf("%w: config version id is required", sharedrepository.ErrInvalidInput)
	}
	var config domain.MonitorConfigVersion
	query := `SELECT ` + configColumns + ` FROM monitor_config_versions WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	if err := repository.queryRow(ctx, query, id).Scan(configScanTargets(&config)...); err != nil {
		return nil, nil, nil, sharedrepository.MapError(err)
	}
	rules, err := repository.rules(ctx, id, lock)
	if err != nil {
		return nil, nil, nil, err
	}
	sources, err := repository.sources(ctx, id, lock)
	if err != nil {
		return nil, nil, nil, err
	}
	return &config, rules, sources, nil
}

func (repository *Repository) CreateDraft(ctx context.Context, config *domain.MonitorConfigVersion, rules []domain.MonitorRule, sources []domain.MonitorSource) error {
	if config == nil || config.MonitorID <= 0 {
		return fmt.Errorf("%w: monitor draft is required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := repository.insertConfig(ctx, transaction.SQL, config); err != nil {
			return err
		}
		if err := repository.insertRules(ctx, transaction.SQL, config.ID, rules); err != nil {
			return err
		}
		return repository.insertSources(ctx, transaction.SQL, config.ID, sources)
	})
}

func (repository *Repository) SaveDraft(ctx context.Context, config *domain.MonitorConfigVersion, rules []domain.MonitorRule, sources []domain.MonitorSource) error {
	if config == nil || config.ID <= 0 || config.Version <= 1 {
		return fmt.Errorf("%w: draft version is required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(ctx, `DELETE FROM monitor_rules WHERE config_version_id = $1`, config.ID); err != nil {
			return sharedrepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `DELETE FROM monitor_sources WHERE config_version_id = $1`, config.ID); err != nil {
			return sharedrepository.MapError(err)
		}
		languages, regions, err := configArrays(config.Config)
		if err != nil {
			return err
		}
		result, err := transaction.SQL.ExecContext(ctx, `UPDATE monitor_config_versions SET timezone = $1, languages = $2::text[], regions = $3::text[], collection_interval_seconds = $4, relevance_threshold = $5, event_threshold = $6, retention_days = $7, version = $8, updated_at = now() WHERE id = $9 AND state = 'draft' AND version = $10`, config.Config.Timezone, languages, regions, config.Config.CollectionIntervalSeconds, config.Config.RelevanceThreshold, config.Config.EventThreshold, config.Config.RetentionDays, config.Version, config.ID, config.Version-1)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sharedrepository.ErrConflict
		}
		if err := repository.insertRules(ctx, transaction.SQL, config.ID, rules); err != nil {
			return err
		}
		return repository.insertSources(ctx, transaction.SQL, config.ID, sources)
	})
}

func (repository *Repository) SaveMonitor(ctx context.Context, monitor *domain.Monitor) error {
	if monitor == nil || monitor.ID <= 0 || monitor.Version <= 1 {
		return fmt.Errorf("%w: monitor version is required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		result, err := transaction.SQL.ExecContext(ctx, `UPDATE monitors SET name = $1, description = $2, status = $3, draft_config_version_id = $4, published_config_version_id = $5, version = $6, updated_at = now() WHERE id = $7 AND version = $8`, monitor.Name, monitor.Description, string(monitor.Status), nullableInt64(monitor.DraftConfigVersionID), nullableInt64(monitor.PublishedConfigVersionID), monitor.Version, monitor.ID, monitor.Version-1)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sharedrepository.ErrConflict
		}
		return nil
	})
}

// SoftDelete hides an archived Monitor from every operational read while
// retaining its immutable configurations and downstream provenance.
func (repository *Repository) SoftDelete(ctx context.Context, monitor *domain.Monitor) error {
	if monitor == nil || monitor.ID <= 0 || monitor.Version <= 1 || monitor.DeletedAt == nil || monitor.Status != domain.MonitorStatusArchived {
		return fmt.Errorf("%w: archived monitor deletion is required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		result, err := transaction.SQL.ExecContext(ctx, `UPDATE monitors SET deleted_at = $1, version = $2, updated_at = now() WHERE id = $3 AND version = $4 AND status = 'archived' AND deleted_at IS NULL`, monitor.DeletedAt, monitor.Version, monitor.ID, monitor.Version-1)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sharedrepository.ErrConflict
		}
		return nil
	})
}

// Publish is deliberately one repository operation so the three immutable
// pointer/state changes cannot accidentally escape the application transaction.
func (repository *Repository) Publish(ctx context.Context, monitor *domain.Monitor, draft, previous *domain.MonitorConfigVersion, sources []domain.MonitorSource) error {
	if monitor == nil || draft == nil || monitor.Version <= 1 || draft.Version <= 1 {
		return fmt.Errorf("%w: publish facts are required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if previous != nil {
			if _, err := transaction.SQL.ExecContext(ctx, `UPDATE monitor_config_versions SET state = 'superseded' WHERE id = $1 AND state = 'published'`, previous.ID); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		for _, source := range sources {
			if _, err := transaction.SQL.ExecContext(ctx, `UPDATE monitor_sources SET query_signature = $1, version = version + 1, updated_at = now() WHERE id = $2 AND config_version_id = $3`, nullableString(source.QuerySignature), source.ID, draft.ID); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		result, err := transaction.SQL.ExecContext(ctx, `UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = $2, version = $3, updated_at = now() WHERE id = $4 AND state = 'draft' AND version = $5`, draft.ConfigHash, nullableTime(draft.PublishedAt), draft.Version, draft.ID, draft.Version-1)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sharedrepository.ErrConflict
		}
		result, err = transaction.SQL.ExecContext(ctx, `UPDATE monitors SET status = 'active', draft_config_version_id = NULL, published_config_version_id = $1, version = $2, updated_at = now() WHERE id = $3 AND version = $4`, draft.ID, monitor.Version, monitor.ID, monitor.Version-1)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sharedrepository.ErrConflict
		}
		return nil
	})
}

func (repository *Repository) ListActivePublished(ctx context.Context) ([]domain.PublishedMonitor, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `SELECT `+monitorColumns+` FROM monitors WHERE status = 'active' AND published_config_version_id IS NOT NULL AND deleted_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	monitors := []domain.Monitor{}
	for rows.Next() {
		var monitor domain.Monitor
		if err := rows.Scan(monitorScanTargets(&monitor)...); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		monitors = append(monitors, monitor)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	result := make([]domain.PublishedMonitor, 0, len(monitors))
	for _, monitor := range monitors {
		config, rules, sources, err := repository.FindConfig(ctx, *monitor.PublishedConfigVersionID)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.PublishedMonitor{Monitor: monitor, Config: *config, Rules: rules, Sources: sources})
	}
	return result, nil
}

func monitorListParameters(query domain.MonitorListQuery) (int, int64, error) {
	limit := query.Limit
	if limit == 0 {
		limit = monitorListDefaultLimit
	}
	if limit < 1 || limit > monitorListMaximumLimit {
		return 0, 0, fmt.Errorf("%w: monitor list limit must be 1-%d", sharedrepository.ErrInvalidInput, monitorListMaximumLimit)
	}
	fingerprint := monitorListFingerprint
	if query.PublishedOnly {
		fingerprint += "-published"
	}
	cursor, err := pagination.Decode(query.Cursor, "id", false, fingerprint)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: monitor cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return limit, cursor.ID, nil
}

func (repository *Repository) insertConfig(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, config *domain.MonitorConfigVersion) error {
	languages, regions, err := configArrays(config.Config)
	if err != nil {
		return err
	}
	if err := queryer.QueryRowContext(ctx, `INSERT INTO monitor_config_versions (monitor_id, revision, state, timezone, languages, regions, collection_interval_seconds, relevance_threshold, event_threshold, retention_days) VALUES ($1, $2, 'draft', $3, $4::text[], $5::text[], $6, $7, $8, $9) RETURNING `+configColumns, config.MonitorID, config.Revision, config.Config.Timezone, languages, regions, config.Config.CollectionIntervalSeconds, config.Config.RelevanceThreshold, config.Config.EventThreshold, config.Config.RetentionDays).Scan(configScanTargets(config)...); err != nil {
		return sharedrepository.MapError(err)
	}
	return nil
}

func (repository *Repository) insertRules(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, configID int64, rules []domain.MonitorRule) error {
	for index := range rules {
		rule := &rules[index]
		rule.ConfigVersionID = configID
		query := `INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, weight, priority, origin, approval_status, enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING ` + ruleColumns
		args := []any{configID, string(rule.RuleType), string(rule.Operator), rule.Value, rule.Weight, rule.Priority, string(rule.Origin), string(rule.ApprovalStatus), rule.Enabled}
		if rule.ID > 0 {
			query = `INSERT INTO monitor_rules (id, config_version_id, rule_type, operator, value, weight, priority, origin, approval_status, enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING ` + ruleColumns
			args = append([]any{rule.ID}, args...)
		}
		if err := queryer.QueryRowContext(ctx, query, args...).Scan(ruleScanTargets(rule)...); err != nil {
			return sharedrepository.MapError(err)
		}
	}
	return nil
}

func (repository *Repository) insertSources(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, configID int64, sources []domain.MonitorSource) error {
	for index := range sources {
		source := &sources[index]
		source.ConfigVersionID = configID
		if err := queryer.QueryRowContext(ctx, `INSERT INTO monitor_sources (config_version_id, source_connection_id, query_override, priority, enabled) VALUES ($1,$2,$3,$4,$5) RETURNING `+monitorSourceColumns, configID, source.SourceConnectionID, nullableString(source.QueryOverride), source.Priority, source.Enabled).Scan(sourceScanTargets(source)...); err != nil {
			return sharedrepository.MapError(err)
		}
	}
	return nil
}

func (repository *Repository) rules(ctx context.Context, configID int64, lock bool) ([]domain.MonitorRule, error) {
	query := `SELECT ` + ruleColumns + ` FROM monitor_rules WHERE config_version_id = $1 ORDER BY id ASC`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := repository.queryRows(ctx, query, configID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	result := []domain.MonitorRule{}
	for rows.Next() {
		var rule domain.MonitorRule
		if err := rows.Scan(ruleScanTargets(&rule)...); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		result = append(result, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return result, nil
}

func (repository *Repository) sources(ctx context.Context, configID int64, lock bool) ([]domain.MonitorSource, error) {
	query := `SELECT ` + monitorSourceColumns + ` FROM monitor_sources WHERE config_version_id = $1 ORDER BY id ASC`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := repository.queryRows(ctx, query, configID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	result := []domain.MonitorSource{}
	for rows.Next() {
		var source domain.MonitorSource
		if err := rows.Scan(sourceScanTargets(&source)...); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		result = append(result, source)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return result, nil
}

func (repository *Repository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
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

func monitorScanTargets(monitor *domain.Monitor) []any {
	return []any{&monitor.ID, &monitor.Version, &monitor.Name, &monitor.Description, &monitor.Status, int64PointerScan{&monitor.DraftConfigVersionID}, int64PointerScan{&monitor.PublishedConfigVersionID}, &monitor.CreatedAt, &monitor.UpdatedAt, timePointerScan{&monitor.DeletedAt}}
}
func configScanTargets(config *domain.MonitorConfigVersion) []any {
	return []any{&config.ID, &config.Version, &config.MonitorID, &config.Revision, &config.State, &config.Config.Timezone, stringSliceScan{&config.Config.Languages}, stringSliceScan{&config.Config.Regions}, &config.Config.CollectionIntervalSeconds, &config.Config.RelevanceThreshold, &config.Config.EventThreshold, &config.Config.RetentionDays, &config.ConfigHash, timePointerScan{&config.PublishedAt}, &config.CreatedAt, &config.UpdatedAt}
}
func ruleScanTargets(rule *domain.MonitorRule) []any {
	return []any{&rule.ID, &rule.Version, &rule.ConfigVersionID, &rule.RuleType, &rule.Operator, &rule.Value, &rule.Weight, &rule.Priority, &rule.Origin, &rule.ApprovalStatus, &rule.Enabled}
}
func sourceScanTargets(source *domain.MonitorSource) []any {
	return []any{&source.ID, &source.Version, &source.ConfigVersionID, &source.SourceConnectionID, nullableStringScan{&source.QueryOverride}, nullableStringScan{&source.QuerySignature}, &source.Priority, &source.Enabled}
}

type int64PointerScan struct{ destination **int64 }

func (scan int64PointerScan) Scan(value any) error {
	if value == nil {
		*scan.destination = nil
		return nil
	}
	var result int64
	switch typed := value.(type) {
	case int64:
		result = typed
	case int32:
		result = int64(typed)
	default:
		return fmt.Errorf("scan nullable int64: %T", value)
	}
	*scan.destination = &result
	return nil
}

type timePointerScan struct{ destination **time.Time }

func (scan timePointerScan) Scan(value any) error {
	if value == nil {
		*scan.destination = nil
		return nil
	}
	instant, ok := value.(time.Time)
	if !ok {
		return fmt.Errorf("scan nullable time: %T", value)
	}
	*scan.destination = &instant
	return nil
}

type nullableStringScan struct{ destination *string }

func (scan nullableStringScan) Scan(value any) error {
	if value == nil {
		*scan.destination = ""
		return nil
	}
	switch typed := value.(type) {
	case string:
		*scan.destination = typed
	case []byte:
		*scan.destination = string(typed)
	default:
		return fmt.Errorf("scan nullable string: %T", value)
	}
	return nil
}

type stringSliceScan struct{ destination *[]string }

func (scan stringSliceScan) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("scan JSON string array: %T", value)
	}
	if err := json.Unmarshal(raw, scan.destination); err != nil {
		return err
	}
	return nil
}

func configArrays(config domain.MonitorConfig) (string, string, error) {
	normalized, err := domain.NormalizeMonitorConfig(config)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return textArray(normalized.Languages), textArray(normalized.Regions), nil
}
func textArray(values []string) string { return "{" + strings.Join(values, ",") + "}" }
func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
func int64Pointer(value int64) *int64 { return &value }
