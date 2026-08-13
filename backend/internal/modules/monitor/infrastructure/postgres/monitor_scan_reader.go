package postgres

import (
	"context"
	"database/sql"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const maxMonitorScans = 100

// MonitorScanReader is a read-only product projection. Monitor owns the
// association join; Source still owns every collection fact it projects.
type MonitorScanReader struct{ runtime *database.Runtime }

var _ sourcedomain.MonitorScanReader = (*MonitorScanReader)(nil)

func NewMonitorScanReader(runtime *database.Runtime) *MonitorScanReader {
	return &MonitorScanReader{runtime: runtime}
}

func (reader *MonitorScanReader) ListMonitorScans(ctx context.Context, monitorID int64, limit int) ([]sourcedomain.MonitorScanSource, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if monitorID <= 0 || limit < 1 || limit > maxMonitorScans {
		return nil, fmt.Errorf("%w: invalid monitor scan query", sharedrepository.ErrInvalidInput)
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
WITH recent_scans AS (
    SELECT collection_run.trigger_type, collection_run.scheduled_at, MAX(collection_run.id) AS latest_run_id
    FROM collection_run_targets AS target
    JOIN collection_runs AS collection_run
      ON collection_run.id = target.collection_run_id
    JOIN monitor_config_versions AS config_version
      ON config_version.id = target.monitor_config_version_id
    WHERE config_version.monitor_id = $1
    GROUP BY collection_run.trigger_type, collection_run.scheduled_at
    ORDER BY latest_run_id DESC
    LIMIT $2
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
JOIN recent_scans
  ON recent_scans.trigger_type = collection_run.trigger_type
 AND recent_scans.scheduled_at = collection_run.scheduled_at
WHERE config_version.monitor_id = $1
ORDER BY recent_scans.latest_run_id DESC, source_connection.id ASC`, monitorID, limit)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()

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
			return nil, databaserepository.MapError(err)
		}
		item.TriggerType = sourcedomain.CollectionTriggerType(triggerType)
		item.Status = sourcedomain.CollectionRunStatus(status)
		item.ErrorCode = errorCode.String
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}
