package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	platformscheduler "github.com/StephenQiu30/hotkey-server/internal/platform/scheduler"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
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
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT
    monitor_source.id,
    config_version.id,
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
    rule.rule_type,
    rule.operator,
    rule.value
FROM monitors AS monitor
JOIN monitor_config_versions AS config_version
  ON config_version.id = monitor.published_config_version_id
JOIN monitor_sources AS monitor_source
  ON monitor_source.config_version_id = config_version.id
JOIN source_connections AS source_connection
  ON source_connection.id = monitor_source.source_connection_id
JOIN source_checkpoints AS checkpoint
  ON checkpoint.monitor_source_id = monitor_source.id
LEFT JOIN monitor_rules AS rule
  ON rule.config_version_id = config_version.id
 AND rule.enabled
 AND rule.approval_status = 'approved'
WHERE monitor.status = 'active'
  AND config_version.state = 'published'
  AND monitor_source.enabled
  AND source_connection.enabled
  AND source_connection.deleted_at IS NULL
  AND monitor_source.query_signature IS NOT NULL
  AND checkpoint.next_poll_at <= $1
ORDER BY monitor_source.id ASC, rule.priority DESC, rule.id ASC`, now.UTC())
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()

	targets := []sourcedomain.PublishedCollectionTarget{}
	var currentID int64
	for rows.Next() {
		row, err := scanPublishedCollectionTarget(rows)
		if err != nil {
			return nil, sharedrepository.MapError(err)
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
		if term, include := collectionTerm(row.ruleType, row.ruleOperator, row.ruleValue); include {
			targets[len(targets)-1].Terms = append(targets[len(targets)-1].Terms, term)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
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
		candidate := platformscheduler.CollectionDueSource{SourceConnectionID: target.SourceConnectionID, ConfigVersionID: target.MonitorConfigVersionID, QuerySignature: target.QuerySignature, NextPollAt: windowStart, CollectionInterval: target.CollectionInterval}
		if existing, ok := byKey[key]; !ok || candidate.ConfigVersionID < existing.ConfigVersionID {
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

type publishedCollectionTargetRow struct {
	monitorSourceID, monitorConfigVersionID, sourceConnectionID int64
	querySignature, queryOverride                               string
	languagesJSON, regionsJSON                                  []byte
	collectionIntervalSeconds                                   int
	checkpointID, checkpointVersion                             int64
	checkpointQueryHash, checkpointCursor, checkpointETag       string
	checkpointLastModified                                      string
	highWatermark, lastFetchedAt                                sql.NullTime
	lastSuccessfulRun                                           sql.NullInt64
	nextPollAt                                                  time.Time
	consecutiveFailures                                         int
	ruleType, ruleOperator, ruleValue                           sql.NullString
}

func scanPublishedCollectionTarget(rows *sql.Rows) (publishedCollectionTargetRow, error) {
	var row publishedCollectionTargetRow
	err := rows.Scan(
		&row.monitorSourceID, &row.monitorConfigVersionID, &row.sourceConnectionID, &row.querySignature, &row.queryOverride,
		&row.languagesJSON, &row.regionsJSON, &row.collectionIntervalSeconds,
		&row.checkpointID, &row.checkpointVersion, &row.checkpointQueryHash, &row.checkpointCursor, &row.checkpointETag,
		&row.checkpointLastModified, &row.highWatermark, &row.lastSuccessfulRun, &row.lastFetchedAt, &row.nextPollAt,
		&row.consecutiveFailures, &row.ruleType, &row.ruleOperator, &row.ruleValue,
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
		MonitorSourceID: row.monitorSourceID, MonitorConfigVersionID: row.monitorConfigVersionID,
		SourceConnectionID: row.sourceConnectionID, QuerySignature: row.querySignature, QueryOverride: row.queryOverride,
		Languages: languages, Regions: regions, CollectionInterval: time.Duration(row.collectionIntervalSeconds) * time.Second,
		Checkpoint: checkpoint,
	}, nil
}

func collectionTerm(ruleType, operator, value sql.NullString) (sourcedomain.CollectionTerm, bool) {
	if !ruleType.Valid || !operator.Valid || !value.Valid {
		return sourcedomain.CollectionTerm{}, false
	}
	switch ruleType.String {
	case "keyword", "phrase", "entity":
		return sourcedomain.CollectionTerm{Value: value.String, Excluded: operator.String == "not_equals"}, true
	case "exclude_keyword":
		return sourcedomain.CollectionTerm{Value: value.String, Excluded: true}, true
	default:
		return sourcedomain.CollectionTerm{}, false
	}
}
