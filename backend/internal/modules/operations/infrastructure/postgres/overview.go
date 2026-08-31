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
	riverQueueLagAlertThresholdSeconds  = 300
	backupRecoveryPointThresholdSeconds = 900
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
	overview.AlertPolicyVersion = operationsdomain.RuntimeAlertPolicyVersion
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
WITH source_auth_candidates AS (
    SELECT DISTINCT ON (source.id)
           source.id AS resource_id,
           target.updated_at AS triggered_at
    FROM source_checkpoints AS checkpoint
    JOIN monitor_sources AS monitor_source
      ON monitor_source.id=checkpoint.monitor_source_id AND monitor_source.enabled
    JOIN source_connections AS source
      ON source.id=monitor_source.source_connection_id
     AND source.enabled AND source.deleted_at IS NULL
    JOIN LATERAL (
        SELECT candidate.error_code,candidate.updated_at
        FROM collection_run_targets AS candidate
        WHERE candidate.monitor_source_id=monitor_source.id
        ORDER BY candidate.updated_at DESC,candidate.id DESC
        LIMIT 1
    ) AS target ON true
    WHERE checkpoint.consecutive_failures >= 3
      AND target.error_code='authentication'
    ORDER BY source.id,target.updated_at ASC
), latest_minio_reconciliation AS (
    SELECT id,scope
    FROM evidence_lineage_reconciliation_runs
    WHERE scope IN ('pg-minio','all') AND status IN ('completed','failed')
    ORDER BY id DESC
    LIMIT 1
), ai_terminal AS (
    SELECT id,status,error_code,created_at,
           row_number() OVER (ORDER BY created_at DESC,id DESC) AS terminal_rank
    FROM ai_runs
    WHERE status IN ('succeeded','failed')
), ai_failure_streak AS (
    SELECT min(id) AS resource_id,min(created_at) AS triggered_at,count(*)::bigint AS affected_count
    FROM ai_terminal
    WHERE terminal_rank <= 3
    HAVING count(*)=3
       AND bool_and(status='failed')
       AND bool_and(error_code IN (70001,70003,70004,70005,70006))
), latest_vault_sync AS (
    SELECT id,conflict_count,COALESCE(finished_at,started_at,created_at) AS triggered_at
    FROM vault_sync_runs
    WHERE status IN ('succeeded','failed')
    ORDER BY id DESC
    LIMIT 1
), latest_backup AS (
    SELECT id,status,recovery_point_at,completed_at
    FROM backup_runs
    ORDER BY id DESC
    LIMIT 1
), candidates AS (
    SELECT 'ALERT-RIVER-JOB-FAILED'::text AS alert_id,
           id AS job_id,
           kind,
           args,
           ''::text AS resource_type,
           0::bigint AS resource_id,
           COALESCE(finalized_at, attempted_at, scheduled_at, created_at) AS triggered_at,
           1::bigint AS affected_weight
    FROM river_job
    WHERE state = 'discarded'
    UNION ALL
    SELECT 'ALERT-RIVER-NO-WORKER'::text AS alert_id,
           id AS job_id,
           kind,
           args,
           ''::text AS resource_type,
           0::bigint AS resource_id,
           scheduled_at AS triggered_at,
           1::bigint AS affected_weight
    FROM river_job
    WHERE state = 'available'
      AND scheduled_at <= now() - make_interval(secs => $1)
    UNION ALL
    SELECT 'ALERT-SOURCE-AUTH',0,'','{}'::jsonb,'source_connection',resource_id,triggered_at,1
    FROM source_auth_candidates
    UNION ALL
    SELECT 'ALERT-MINIO-WRITE',0,'','{}'::jsonb,'evidence_reconciliation',run.id,item.created_at,1
    FROM latest_minio_reconciliation AS run
    JOIN evidence_lineage_reconciliation_items AS item ON item.run_id=run.id AND item.scope=run.scope
    WHERE item.asset_type IN ('evidence_snapshot','raw_object_orphan')
      AND item.finding IN ('missing','digest_mismatch')
    UNION ALL
    SELECT 'ALERT-CODEX-FAILURE',0,'','{}'::jsonb,'ai_run',resource_id,triggered_at,affected_count
    FROM ai_failure_streak
    UNION ALL
    SELECT 'ALERT-VAULT-CONFLICT',0,'','{}'::jsonb,'vault_sync_run',id,triggered_at,conflict_count
    FROM latest_vault_sync
    WHERE conflict_count > 0
    UNION ALL
	SELECT 'ALERT-BACKUP-FAILED',0,'','{}'::jsonb,'backup_run',id,completed_at,1
	FROM latest_backup
	WHERE status='failed'
	   OR recovery_point_at < now() - make_interval(secs => $6)
	UNION ALL
    SELECT 'ALERT-SEARCH-BACKLOG',id,kind,args,'river_job',id,scheduled_at,1
    FROM river_job
    WHERE state='available'
      AND scheduled_at <= now() - make_interval(secs => $1)
      AND kind IN ($2,$3,$4,$5)
), ranked AS (
    SELECT candidates.*,
           sum(affected_weight) OVER (PARTITION BY alert_id) AS affected_count,
           row_number() OVER (PARTITION BY alert_id ORDER BY triggered_at ASC, job_id ASC) AS alert_rank
    FROM candidates
)
SELECT alert_id,job_id,kind,args,resource_type,resource_id,triggered_at,affected_count
FROM ranked
WHERE alert_rank = 1
ORDER BY CASE alert_id
    WHEN 'ALERT-RIVER-JOB-FAILED' THEN 1
    WHEN 'ALERT-RIVER-NO-WORKER' THEN 2
    WHEN 'ALERT-SOURCE-AUTH' THEN 3
    WHEN 'ALERT-MINIO-WRITE' THEN 4
    WHEN 'ALERT-CODEX-FAILURE' THEN 5
    WHEN 'ALERT-VAULT-CONFLICT' THEN 6
    WHEN 'ALERT-BACKUP-FAILED' THEN 7
    WHEN 'ALERT-SEARCH-BACKLOG' THEN 8
    ELSE 9
END`, riverQueueLagAlertThresholdSeconds, queue.KindProjectKnowledge, queue.KindReconcileKnowledge,
		queue.KindGenerateSourceDocument, queue.KindProjectAcceptedDocumentMatch, backupRecoveryPointThresholdSeconds)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()

	alerts := make([]operationsdomain.RuntimeAlert, 0, 8)
	for rows.Next() {
		var alert operationsdomain.RuntimeAlert
		var kind string
		var args json.RawMessage
		if err := rows.Scan(
			&alert.AlertID, &alert.JobID, &kind, &args, &alert.ResourceType, &alert.ResourceID,
			&alert.TriggeredAt, &alert.AffectedCount,
		); err != nil {
			return nil, databaserepository.MapError(err)
		}
		alert.EventID, alert.TraceID = safeEventCorrelation(kind, args)
		alert, found := operationsdomain.ApplyRuntimeAlertPolicy(alert)
		if !found {
			return nil, sharedrepository.ErrConstraint
		}
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
