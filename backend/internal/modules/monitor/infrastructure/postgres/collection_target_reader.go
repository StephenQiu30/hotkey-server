package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	platformscheduler "github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// PublishedCollectionTargetReader is the Monitor-owned read adapter for
// Source collection planning. It reads only immutable published configuration,
// enabled Monitor associations and their checkpoints. The SourceConnection
// join is eligibility-only (enabled and not archived); no SourceConnection
// fields other than the association's ID are projected. It neither exposes
// Monitor records nor creates or changes collection facts.
type PublishedCollectionTargetReader struct{ runtime *database.Runtime }

var _ sourcedomain.PublishedCollectionTargetReader = (*PublishedCollectionTargetReader)(nil)

func NewPublishedCollectionTargetReader(runtime *database.Runtime) *PublishedCollectionTargetReader {
	return &PublishedCollectionTargetReader{runtime: runtime}
}

func (reader *PublishedCollectionTargetReader) ListDue(ctx context.Context, now time.Time) ([]sourcedomain.PublishedCollectionTarget, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if now.IsZero() {
		return nil, fmt.Errorf("%w: collection due time is required", sharedrepository.ErrInvalidInput)
	}
	return reader.listPublishedTargets(ctx, "checkpoint.next_poll_at <= $1", now.UTC())
}

// ListForManualCollection returns only the active published targets belonging
// to the requested Monitor. It intentionally ignores checkpoint due time: the
// API uses this projection only to submit bounded durable jobs, never to fetch.
func (reader *PublishedCollectionTargetReader) ListForManualCollection(ctx context.Context, monitorID int64) ([]sourcedomain.PublishedCollectionTarget, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if monitorID <= 0 {
		return nil, fmt.Errorf("%w: monitor id is required", sharedrepository.ErrInvalidInput)
	}
	targets, err := reader.listPublishedTargets(ctx, "monitor.id = $1", monitorID)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: active published monitor targets not found", sharedrepository.ErrNotFound)
	}
	return targets, nil
}

func (reader *PublishedCollectionTargetReader) listPublishedTargets(ctx context.Context, predicate string, args ...any) ([]sourcedomain.PublishedCollectionTarget, error) {
	queryer := interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	}(reader.runtime.SQL)
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		queryer = transaction.SQL
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT
    monitor.id,
    monitor_source.id,
    config_version.id,
    compiled_profile.id,
    monitor_source.source_connection_id,
    monitor_source.query_signature,
    COALESCE(monitor_source.query_override, ''),
    array_to_json(config_version.languages),
    array_to_json(config_version.regions),
    config_version.collection_interval_seconds,
    checkpoint.id,
    checkpoint.version,
    checkpoint.query_hash,
    COALESCE(checkpoint.cursor_value, ''),
    COALESCE(checkpoint.etag, ''),
    COALESCE(checkpoint.last_modified, ''),
    checkpoint.high_watermark,
    checkpoint.last_successful_run_id,
    checkpoint.last_fetched_at,
    checkpoint.next_poll_at,
    checkpoint.consecutive_failures,
    term.value,
    term.excluded
FROM monitors AS monitor
JOIN monitor_config_versions AS config_version
  ON config_version.id = monitor.published_config_version_id
JOIN monitor_sources AS monitor_source
  ON monitor_source.config_version_id = config_version.id
JOIN source_connections AS source_connection
  ON source_connection.id = monitor_source.source_connection_id
JOIN source_checkpoints AS checkpoint
  ON checkpoint.monitor_source_id = monitor_source.id
JOIN monitor_compiled_profiles AS compiled_profile
  ON compiled_profile.monitor_id=monitor.id
 AND compiled_profile.purpose='published'
 AND compiled_profile.monitor_version_id=config_version.id
 AND compiled_profile.status='ready'
LEFT JOIN LATERAL (
    SELECT selected.value,selected.excluded,selected.term_order,selected.ordinal
    FROM (
        SELECT DISTINCT ON (candidate.excluded,candidate.normalized_value)
               candidate.value,candidate.excluded,candidate.term_order,candidate.ordinal
        FROM (
            SELECT clause.value,(clause.operator='must_not') AS excluded,
                   clause.normalized_value,0 AS term_order,clause.ordinal::bigint AS ordinal
            FROM monitor_compiled_clauses AS clause
            WHERE clause.compiled_profile_id=compiled_profile.id
              AND clause.field IN ('term','phrase','action','location')
            UNION ALL
            SELECT alias.alias,false,alias.normalized_alias,1,alias.ordinal::bigint
            FROM monitor_compiled_entity_aliases AS alias
            WHERE alias.compiled_profile_id=compiled_profile.id
        ) AS candidate
        ORDER BY candidate.excluded,candidate.normalized_value,candidate.term_order,candidate.ordinal
    ) AS selected
    ORDER BY selected.term_order,selected.ordinal,selected.value
) AS term ON true
WHERE monitor.status = 'active'
  AND config_version.state = 'published'
  AND monitor_source.enabled
  AND source_connection.enabled
  AND source_connection.deleted_at IS NULL
  AND monitor_source.query_signature IS NOT NULL
  AND `+predicate+`
ORDER BY monitor_source.id ASC, term.term_order ASC, term.ordinal ASC, term.value ASC`, args...)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()

	targets := []sourcedomain.PublishedCollectionTarget{}
	var currentID int64
	for rows.Next() {
		row, err := scanPublishedCollectionTarget(rows)
		if err != nil {
			return nil, databaserepository.MapError(err)
		}
		if row.monitorSourceID != currentID {
			if currentID != 0 {
				if err := targets[len(targets)-1].Validate(); err != nil {
					return nil, fmt.Errorf("%w: invalid published collection target: %v", sharedrepository.ErrConstraint, err)
				}
			}
			currentID = row.monitorSourceID
			target, err := row.target()
			if err != nil {
				return nil, fmt.Errorf("%w: decode published collection target: %v", sharedrepository.ErrConstraint, err)
			}
			targets = append(targets, target)
		}
		if term, include := collectionTerm(row.termValue, row.termExcluded); include {
			targets[len(targets)-1].Terms = append(targets[len(targets)-1].Terms, term)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	if currentID != 0 {
		if err := targets[len(targets)-1].Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid published collection target: %v", sharedrepository.ErrConstraint, err)
		}
	}
	return targets, nil
}

// ListDueCollections converts the immutable Monitor-owned target projection
// into the scheduler's small source/signature/window envelope. Multiple
// Monitor targets sharing one source and signature become one collect job;
// the worker re-reads all target facts before executing the collection.
func (reader *PublishedCollectionTargetReader) ListDueCollections(ctx context.Context, now time.Time) ([]platformscheduler.CollectionDueSource, error) {
	targets, err := reader.ListDue(ctx, now)
	if err != nil {
		return nil, err
	}
	type collectionKey struct {
		sourceID, windowStart, windowEnd int64
		signature                        string
	}
	byKey := make(map[collectionKey]platformscheduler.CollectionDueSource, len(targets))
	for _, target := range targets {
		windowStart := target.Checkpoint.NextPollAt.UTC()
		windowEnd := windowStart.Add(target.CollectionInterval)
		key := collectionKey{sourceID: target.SourceConnectionID, signature: target.QuerySignature, windowStart: windowStart.UnixNano(), windowEnd: windowEnd.UnixNano()}
		candidate := platformscheduler.CollectionDueSource{
			MonitorID: target.MonitorID, MonitorVersionID: target.MonitorConfigVersionID,
			CompiledProfileID: target.CompiledProfileID, SourceConnectionID: target.SourceConnectionID,
			QuerySignature: target.QuerySignature, NextPollAt: windowStart, CollectionInterval: target.CollectionInterval,
		}
		if existing, ok := byKey[key]; !ok || candidate.MonitorVersionID < existing.MonitorVersionID {
			byKey[key] = candidate
		}
	}
	result := make([]platformscheduler.CollectionDueSource, 0, len(byKey))
	for _, source := range byKey {
		result = append(result, source)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].SourceConnectionID != result[right].SourceConnectionID {
			return result[left].SourceConnectionID < result[right].SourceConnectionID
		}
		if result[left].QuerySignature != result[right].QuerySignature {
			return result[left].QuerySignature < result[right].QuerySignature
		}
		return result[left].NextPollAt.Before(result[right].NextPollAt)
	})
	return result, nil
}

// ListForCollection re-reads the published target projection for one durable
// collect_source envelope. It intentionally reuses the same eligibility query
// as Cron, so a target paused or unpublished after enqueue is not executed.
func (reader *PublishedCollectionTargetReader) ListForCollection(ctx context.Context, sourceConnectionID, configVersionID int64, querySignature string, windowStart, windowEnd time.Time, triggerType sourcedomain.CollectionTriggerType) ([]sourcedomain.PublishedCollectionTarget, error) {
	if sourceConnectionID <= 0 || configVersionID <= 0 || querySignature == "" || windowStart.IsZero() || windowEnd.IsZero() || !windowEnd.After(windowStart) {
		return nil, fmt.Errorf("invalid collection envelope")
	}
	var (
		targets []sourcedomain.PublishedCollectionTarget
		err     error
	)
	retryableWindow := false
	if triggerType != sourcedomain.CollectionTriggerManual {
		retryableWindow, err = reader.retryableCollectionWindowDue(ctx, sourceConnectionID, querySignature, windowStart, windowEnd, triggerType)
		if err != nil {
			return nil, err
		}
	}
	if triggerType == sourcedomain.CollectionTriggerManual || retryableWindow {
		targets, err = reader.listPublishedTargets(ctx, "monitor_source.source_connection_id = $1 AND monitor_source.query_signature = $2", sourceConnectionID, querySignature)
	} else {
		targets, err = reader.ListDue(ctx, windowEnd.UTC())
	}
	if err != nil {
		return nil, err
	}
	matched := make([]sourcedomain.PublishedCollectionTarget, 0, len(targets))
	for _, target := range targets {
		if target.SourceConnectionID != sourceConnectionID || target.QuerySignature != querySignature {
			continue
		}
		if triggerType != sourcedomain.CollectionTriggerManual && !retryableWindow && !target.Checkpoint.NextPollAt.UTC().Equal(windowStart.UTC()) {
			continue
		}
		// The scheduler stores the smallest config version for a shared source
		// window. Keep all matching immutable targets, including that selected
		// version, so Source can build one shared request and persist every target.
		if target.MonitorConfigVersionID > 0 {
			matched = append(matched, target)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("%w: collection target not found", sharedrepository.ErrNotFound)
	}
	return matched, nil
}

func (reader *PublishedCollectionTargetReader) retryableCollectionWindowDue(ctx context.Context, sourceConnectionID int64, querySignature string, windowStart, windowEnd time.Time, triggerType sourcedomain.CollectionTriggerType) (bool, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return false, sharedrepository.ErrUnavailable
	}
	var due bool
	err := reader.runtime.SQL.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM collection_runs
  WHERE source_connection_id=$1 AND query_signature=$2 AND window_start=$3 AND window_end=$4
    AND trigger_type=$5 AND status='failed' AND error_code IN ('rate_limited','temporary')
    AND (retry_after IS NULL OR retry_after <= now())
)`, sourceConnectionID, querySignature, windowStart.UTC(), windowEnd.UTC(), string(triggerType)).Scan(&due)
	if err != nil {
		return false, databaserepository.MapError(err)
	}
	return due, nil
}

type publishedCollectionTargetRow struct {
	monitorID, monitorSourceID, monitorConfigVersionID    int64
	compiledProfileID, sourceConnectionID                 int64
	querySignature, queryOverride                         string
	languagesJSON, regionsJSON                            []byte
	collectionIntervalSeconds                             int
	checkpointID, checkpointVersion                       int64
	checkpointQueryHash, checkpointCursor, checkpointETag string
	checkpointLastModified                                string
	highWatermark, lastFetchedAt                          sql.NullTime
	lastSuccessfulRun                                     sql.NullInt64
	nextPollAt                                            time.Time
	consecutiveFailures                                   int
	termValue                                             sql.NullString
	termExcluded                                          sql.NullBool
}

func scanPublishedCollectionTarget(rows *sql.Rows) (publishedCollectionTargetRow, error) {
	var row publishedCollectionTargetRow
	err := rows.Scan(
		&row.monitorID, &row.monitorSourceID, &row.monitorConfigVersionID, &row.compiledProfileID,
		&row.sourceConnectionID, &row.querySignature, &row.queryOverride,
		&row.languagesJSON, &row.regionsJSON, &row.collectionIntervalSeconds,
		&row.checkpointID, &row.checkpointVersion, &row.checkpointQueryHash, &row.checkpointCursor, &row.checkpointETag,
		&row.checkpointLastModified, &row.highWatermark, &row.lastSuccessfulRun, &row.lastFetchedAt, &row.nextPollAt,
		&row.consecutiveFailures, &row.termValue, &row.termExcluded,
	)
	return row, err
}

func (row publishedCollectionTargetRow) target() (sourcedomain.PublishedCollectionTarget, error) {
	var languages, regions []string
	if err := json.Unmarshal(row.languagesJSON, &languages); err != nil {
		return sourcedomain.PublishedCollectionTarget{}, fmt.Errorf("decode languages: %w", err)
	}
	if err := json.Unmarshal(row.regionsJSON, &regions); err != nil {
		return sourcedomain.PublishedCollectionTarget{}, fmt.Errorf("decode regions: %w", err)
	}
	checkpoint := sourcedomain.CollectionCheckpoint{
		ID: row.checkpointID, Version: row.checkpointVersion, MonitorSourceID: row.monitorSourceID,
		QueryHash: row.checkpointQueryHash, CursorValue: row.checkpointCursor, ETag: row.checkpointETag,
		LastModified: row.checkpointLastModified, NextPollAt: row.nextPollAt, ConsecutiveFailures: row.consecutiveFailures,
	}
	if row.highWatermark.Valid {
		value := row.highWatermark.Time.UTC()
		checkpoint.HighWatermark = &value
	}
	if row.lastSuccessfulRun.Valid {
		value := row.lastSuccessfulRun.Int64
		checkpoint.LastSuccessfulRunID = &value
	}
	if row.lastFetchedAt.Valid {
		value := row.lastFetchedAt.Time.UTC()
		checkpoint.LastFetchedAt = &value
	}
	return sourcedomain.PublishedCollectionTarget{
		MonitorID: row.monitorID, MonitorSourceID: row.monitorSourceID,
		MonitorConfigVersionID: row.monitorConfigVersionID, CompiledProfileID: row.compiledProfileID,
		SourceConnectionID: row.sourceConnectionID, QuerySignature: row.querySignature, QueryOverride: row.queryOverride,
		Languages: languages, Regions: regions, CollectionInterval: time.Duration(row.collectionIntervalSeconds) * time.Second,
		Checkpoint: checkpoint,
	}, nil
}

func collectionTerm(value sql.NullString, excluded sql.NullBool) (sourcedomain.CollectionTerm, bool) {
	if !value.Valid || !excluded.Valid {
		return sourcedomain.CollectionTerm{}, false
	}
	return sourcedomain.CollectionTerm{Value: value.String, Excluded: excluded.Bool}, true
}
