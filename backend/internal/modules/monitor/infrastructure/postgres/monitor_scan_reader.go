package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	defaultMonitorScans      = 20
	maxMonitorScans          = 100
	monitorScanCursorVersion = 1
	monitorScanCursorPurpose = "monitor_scan_list"
)

// MonitorScanReader is a read-only product projection. Monitor owns the
// association join; Source still owns every collection fact it projects.
type MonitorScanReader struct {
	runtime     *database.Runtime
	cursorCodec *pagination.Codec
}

type monitorScanCursor struct {
	Version       int   `json:"v"`
	MonitorID     int64 `json:"monitor_id"`
	SnapshotRunID int64 `json:"snapshot_run_id"`
	AfterRunID    int64 `json:"after_run_id"`
}

type monitorScanBoundary struct {
	TriggerType sourcedomain.CollectionTriggerType
	ScheduledAt time.Time
	LatestRunID int64
}

var _ sourcedomain.MonitorScanReader = (*MonitorScanReader)(nil)

func NewMonitorScanReader(runtime *database.Runtime) *MonitorScanReader {
	seed := "monitor-scan:unavailable"
	if runtime != nil && runtime.Pool != nil {
		seed = "monitor-scan:" + runtime.Pool.Config().ConnString()
	}
	return NewMonitorScanReaderWithCursorCodec(runtime, pagination.NewTestCodec(seed))
}

func NewMonitorScanReaderWithCursorCodec(runtime *database.Runtime, codec *pagination.Codec) *MonitorScanReader {
	return &MonitorScanReader{runtime: runtime, cursorCodec: codec}
}

func (reader *MonitorScanReader) ListMonitorScans(ctx context.Context, query sourcedomain.MonitorScanListQuery) (sourcedomain.MonitorScanSourcePage, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil || reader.cursorCodec == nil {
		return sourcedomain.MonitorScanSourcePage{}, sharedrepository.ErrUnavailable
	}
	limit, cursor, err := reader.monitorScanParameters(ctx, query)
	if err != nil {
		return sourcedomain.MonitorScanSourcePage{}, err
	}
	if cursor.SnapshotRunID == 0 {
		return sourcedomain.MonitorScanSourcePage{Items: []sourcedomain.MonitorScanSource{}}, nil
	}
	boundaries, err := reader.monitorScanBoundaries(ctx, cursor, limit+1)
	if err != nil {
		return sourcedomain.MonitorScanSourcePage{}, err
	}
	if len(boundaries) == 0 {
		return sourcedomain.MonitorScanSourcePage{Items: []sourcedomain.MonitorScanSource{}}, nil
	}
	pageAfterRunID := cursor.AfterRunID
	var nextCursor string
	if len(boundaries) > limit {
		boundaries = boundaries[:limit]
		cursor.AfterRunID = boundaries[len(boundaries)-1].LatestRunID
		nextCursor, err = reader.cursorCodec.Seal(monitorScanCursorPurpose, cursor)
		if err != nil {
			return sourcedomain.MonitorScanSourcePage{}, fmt.Errorf("%w: encode monitor scan cursor", sharedrepository.ErrInvalidInput)
		}
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
WITH selected_scans AS (
    SELECT collection_run.trigger_type, collection_run.scheduled_at, MAX(collection_run.id) AS latest_run_id
    FROM collection_run_targets AS target
    JOIN collection_runs AS collection_run
      ON collection_run.id = target.collection_run_id
    JOIN monitor_config_versions AS config_version
      ON config_version.id = target.monitor_config_version_id
    WHERE config_version.monitor_id = $1
      AND collection_run.id <= $2
    GROUP BY collection_run.trigger_type, collection_run.scheduled_at
    HAVING $3::bigint = 0 OR MAX(collection_run.id) < $3
    ORDER BY latest_run_id DESC
    LIMIT $4
)
SELECT
    collection_run.id,
    config_version.monitor_id,
    source_connection.id,
    source_connection.name,
    source_connection.source_type,
    collection_run.trigger_type,
    target.target_status,
    target.candidate_count,
    target.accepted_count,
    target.rejected_count,
    target.error_code,
    collection_run.scheduled_at,
    collection_run.started_at,
    collection_run.finished_at
FROM collection_run_targets AS target
JOIN collection_runs AS collection_run
  ON collection_run.id = target.collection_run_id
JOIN monitor_config_versions AS config_version
  ON config_version.id = target.monitor_config_version_id
JOIN source_connections AS source_connection
  ON source_connection.id = collection_run.source_connection_id
JOIN selected_scans
  ON selected_scans.trigger_type = collection_run.trigger_type
 AND selected_scans.scheduled_at = collection_run.scheduled_at
WHERE config_version.monitor_id = $1
  AND collection_run.id <= $2
ORDER BY selected_scans.latest_run_id DESC, source_connection.id ASC, collection_run.id ASC`, query.MonitorID, cursor.SnapshotRunID, pageAfterRunID, len(boundaries))
	if err != nil {
		return sourcedomain.MonitorScanSourcePage{}, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]sourcedomain.MonitorScanSource, 0, limit)
	for rows.Next() {
		var item sourcedomain.MonitorScanSource
		var triggerType, status string
		var errorCode sql.NullString
		if err := rows.Scan(
			&item.RunID, &item.MonitorID, &item.SourceConnectionID,
			&item.SourceName, &item.SourceType, &triggerType, &status,
			&item.CandidateCount, &item.AcceptedCount, &item.RejectedCount,
			&errorCode, &item.ScheduledAt, &item.StartedAt, &item.FinishedAt,
		); err != nil {
			return sourcedomain.MonitorScanSourcePage{}, databaserepository.MapError(err)
		}
		item.TriggerType = sourcedomain.CollectionTriggerType(triggerType)
		item.Status = sourcedomain.CollectionRunStatus(status)
		item.ErrorCode = errorCode.String
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return sourcedomain.MonitorScanSourcePage{}, databaserepository.MapError(err)
	}
	return sourcedomain.MonitorScanSourcePage{Items: items, NextCursor: nextCursor}, nil
}

func (reader *MonitorScanReader) monitorScanParameters(ctx context.Context, query sourcedomain.MonitorScanListQuery) (int, monitorScanCursor, error) {
	limit := query.Limit
	if limit == 0 {
		limit = defaultMonitorScans
	}
	if query.MonitorID <= 0 || limit < 1 || limit > maxMonitorScans {
		return 0, monitorScanCursor{}, fmt.Errorf("%w: invalid monitor scan query", sharedrepository.ErrInvalidInput)
	}
	cursor := monitorScanCursor{Version: monitorScanCursorVersion, MonitorID: query.MonitorID}
	if query.Cursor != "" {
		if err := reader.cursorCodec.Open(query.Cursor, monitorScanCursorPurpose, &cursor); err != nil ||
			cursor.Version != monitorScanCursorVersion || cursor.MonitorID != query.MonitorID ||
			cursor.SnapshotRunID <= 0 || cursor.AfterRunID <= 0 || cursor.AfterRunID > cursor.SnapshotRunID {
			return 0, monitorScanCursor{}, fmt.Errorf("%w: invalid monitor scan cursor", sharedrepository.ErrInvalidInput)
		}
		return limit, cursor, nil
	}
	if err := reader.runtime.SQL.QueryRowContext(ctx, `
SELECT COALESCE(MAX(collection_run.id), 0)
FROM collection_run_targets AS target
JOIN collection_runs AS collection_run ON collection_run.id = target.collection_run_id
JOIN monitor_config_versions AS config_version ON config_version.id = target.monitor_config_version_id
WHERE config_version.monitor_id = $1`, query.MonitorID).Scan(&cursor.SnapshotRunID); err != nil {
		return 0, monitorScanCursor{}, databaserepository.MapError(err)
	}
	return limit, cursor, nil
}

func (reader *MonitorScanReader) monitorScanBoundaries(ctx context.Context, cursor monitorScanCursor, limit int) ([]monitorScanBoundary, error) {
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT collection_run.trigger_type, collection_run.scheduled_at, MAX(collection_run.id) AS latest_run_id
FROM collection_run_targets AS target
JOIN collection_runs AS collection_run ON collection_run.id = target.collection_run_id
JOIN monitor_config_versions AS config_version ON config_version.id = target.monitor_config_version_id
WHERE config_version.monitor_id = $1
  AND collection_run.id <= $2
GROUP BY collection_run.trigger_type, collection_run.scheduled_at
HAVING $3::bigint = 0 OR MAX(collection_run.id) < $3
ORDER BY latest_run_id DESC
LIMIT $4`, cursor.MonitorID, cursor.SnapshotRunID, cursor.AfterRunID, limit)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]monitorScanBoundary, 0, limit)
	for rows.Next() {
		var item monitorScanBoundary
		if err := rows.Scan(&item.TriggerType, &item.ScheduledAt, &item.LatestRunID); err != nil {
			return nil, databaserepository.MapError(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}
