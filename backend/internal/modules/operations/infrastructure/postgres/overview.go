package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ operationsapplication.OverviewStore = (*JobRepository)(nil)

const (
	riverQueueLagAlertThresholdSeconds = 300
	riverAlertRunbookURL               = "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#river-alert-response"
)

func (repository *JobRepository) RuntimeOverview(ctx context.Context) (operationsdomain.RuntimeOverview, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return operationsdomain.RuntimeOverview{}, sharedrepository.ErrUnavailable
	}
	var overview operationsdomain.RuntimeOverview
	var oldest sql.NullTime
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE state = 'available'),
       count(*) FILTER (WHERE state = 'running'),
       count(*) FILTER (WHERE state = 'completed'),
       count(*) FILTER (WHERE state = 'discarded'),
       count(*) FILTER (WHERE state = 'cancelled'),
       min(scheduled_at) FILTER (WHERE state = 'available'),
       coalesce(greatest(extract(epoch FROM now() - min(scheduled_at) FILTER (WHERE state = 'available' AND scheduled_at <= now())), 0), 0)
FROM river_job`).Scan(
		&overview.AvailableJobs, &overview.RunningJobs, &overview.CompletedJobs,
		&overview.DiscardedJobs, &overview.CancelledJobs, &oldest, &overview.QueueLagSeconds,
	)
	if err != nil {
		return operationsdomain.RuntimeOverview{}, databaserepository.MapError(err)
	}
	if oldest.Valid {
		value := oldest.Time.UTC()
		overview.OldestAvailableAt = &value
	}
	overview.Alerts, err = repository.runtimeAlerts(ctx)
	if err != nil {
		return operationsdomain.RuntimeOverview{}, err
	}
	if err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT now()`).Scan(&overview.GeneratedAt); err != nil {
		return operationsdomain.RuntimeOverview{}, databaserepository.MapError(err)
	}
	return overview, nil
}

func (repository *JobRepository) runtimeAlerts(ctx context.Context) ([]operationsdomain.RuntimeAlert, error) {
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
WITH candidates AS (
    SELECT 'ALERT-RIVER-JOB-FAILED'::text AS alert_id,
           'river_job_discarded'::text AS reason_code,
           id AS job_id,
           kind,
           args,
           COALESCE(finalized_at, attempted_at, scheduled_at, created_at) AS triggered_at
    FROM river_job
    WHERE state = 'discarded'
    UNION ALL
    SELECT 'ALERT-RIVER-NO-WORKER'::text AS alert_id,
           'river_queue_lag_exceeded'::text AS reason_code,
           id AS job_id,
           kind,
           args,
           scheduled_at AS triggered_at
    FROM river_job
    WHERE state = 'available'
      AND scheduled_at <= now() - make_interval(secs => $1)
), ranked AS (
    SELECT candidates.*,
           count(*) OVER (PARTITION BY alert_id) AS affected_count,
           row_number() OVER (PARTITION BY alert_id ORDER BY triggered_at ASC, job_id ASC) AS alert_rank
    FROM candidates
)
SELECT alert_id, reason_code, job_id, kind, args, triggered_at, affected_count
FROM ranked
WHERE alert_rank = 1
ORDER BY CASE alert_id
    WHEN 'ALERT-RIVER-JOB-FAILED' THEN 1
    WHEN 'ALERT-RIVER-NO-WORKER' THEN 2
    ELSE 3
END`, riverQueueLagAlertThresholdSeconds)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()

	alerts := make([]operationsdomain.RuntimeAlert, 0, 2)
	for rows.Next() {
		var alert operationsdomain.RuntimeAlert
		var kind string
		var args json.RawMessage
		if err := rows.Scan(&alert.AlertID, &alert.ReasonCode, &alert.JobID, &kind, &args, &alert.TriggeredAt, &alert.AffectedCount); err != nil {
			return nil, databaserepository.MapError(err)
		}
		alert.Severity = "p1"
		alert.RunbookURL = riverAlertRunbookURL
		alert.EventID, alert.TraceID = safeEventCorrelation(kind, args)
		alerts = append(alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return alerts, nil
}

func safeEventCorrelation(kind string, args json.RawMessage) (int64, string) {
	if kind != queue.KindExtractAutomaticClaimEvidence && kind != queue.KindRefreshProductEvent {
		return 0, ""
	}
	var value struct {
		MicroEventID int64  `json:"micro_event_id"`
		TraceID      string `json:"trace_id"`
	}
	if err := json.Unmarshal(args, &value); err != nil || value.MicroEventID <= 0 {
		return 0, ""
	}
	if !validTraceID(value.TraceID) {
		value.TraceID = ""
	}
	return value.MicroEventID, value.TraceID
}

func validTraceID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}

var _ interface {
	RuntimeOverview(context.Context) (operationsdomain.RuntimeOverview, error)
} = (*JobRepository)(nil)
