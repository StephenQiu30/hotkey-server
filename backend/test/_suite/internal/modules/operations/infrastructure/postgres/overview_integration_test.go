package postgres_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestJobRepositoryRuntimeOverviewCountsSafeQueueStates(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := queue.NewStore(runtime)
	if _, _, err := store.Enqueue(ctx, queue.Job{Kind: queue.KindNormalizeContent, UniqueKey: "overview-a", Payload: queue.Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 2, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	overview, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.AvailableJobs != 1 || overview.RunningJobs != 0 || overview.OldestAvailableAt == nil || overview.GeneratedAt.IsZero() {
		t.Fatalf("overview = %#v", overview)
	}
	if overview.Alerts == nil || len(overview.Alerts) != 0 {
		t.Fatalf("healthy alerts = %#v", overview.Alerts)
	}
}

func TestJobRepositoryRuntimeOverviewProjectsActionableRiverAlertsWithSafeCorrelation(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	const traceID = "0123456789abcdef0123456789abcdef"
	store := queue.NewStore(runtime)
	now := time.Now().UTC()
	oldJobID, _, err := store.Enqueue(ctx, queue.Job{
		Kind:        queue.KindExtractAutomaticClaimEvidence,
		UniqueKey:   "overview-overdue-event-job",
		DurableArgs: json.RawMessage(`{"micro_event_id":41,"document_version_id":2,"trace_id":"` + traceID + `","secret":"must-not-leak"}`),
		ScheduledAt: now.Add(-6 * time.Minute), MaxAttempts: 2, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	discardedJobID, _, err := store.Enqueue(ctx, queue.Job{
		Kind:        queue.KindRefreshProductEvent,
		UniqueKey:   "overview-discarded-event-job",
		DurableArgs: json.RawMessage(`{"micro_event_id":42,"trace_id":"` + traceID + `","prompt":"must-not-leak"}`),
		ScheduledAt: now, MaxAttempts: 2, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state='discarded',attempt=2,finalized_at=now() WHERE id=$1`, discardedJobID); err != nil {
		t.Fatal(err)
	}
	extraBacklogID, _, err := store.Enqueue(ctx, queue.Job{
		Kind: queue.KindNormalizeContent, UniqueKey: "overview-overdue-generic-job",
		Payload: queue.Payload{EntityID: 7, EntityVersion: 1}, ScheduledAt: now.Add(-330 * time.Second), MaxAttempts: 2, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	extraDiscardedID, _, err := store.Enqueue(ctx, queue.Job{
		Kind: queue.KindBuildReport, UniqueKey: "overview-discarded-generic-job",
		Payload: queue.Payload{EntityID: 8, EntityVersion: 1}, ScheduledAt: now, MaxAttempts: 2, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state='discarded',attempt=2,finalized_at=now() + interval '1 second' WHERE id=$1`, extraDiscardedID); err != nil {
		t.Fatal(err)
	}

	overview, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Alerts) != 2 {
		t.Fatalf("alerts = %#v", overview.Alerts)
	}
	failure, backlog := overview.Alerts[0], overview.Alerts[1]
	if failure.AlertID != "ALERT-RIVER-JOB-FAILED" || failure.Severity != "p1" || failure.ReasonCode != "river_job_discarded" ||
		failure.JobID != discardedJobID || failure.EventID != 42 || failure.TraceID != traceID || failure.AffectedCount != 2 ||
		failure.TriggeredAt.IsZero() || !strings.Contains(failure.RunbookURL, "004-") {
		t.Fatalf("failure alert = %#v", failure)
	}
	if backlog.AlertID != "ALERT-RIVER-NO-WORKER" || backlog.Severity != "p1" || backlog.ReasonCode != "river_queue_lag_exceeded" ||
		backlog.JobID != oldJobID || backlog.EventID != 41 || backlog.TraceID != traceID || backlog.AffectedCount != 2 ||
		backlog.TriggeredAt.IsZero() || !strings.Contains(backlog.RunbookURL, "004-") {
		t.Fatalf("backlog alert = %#v", backlog)
	}
	encoded, err := json.Marshal(overview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-leak") || strings.Contains(string(encoded), "prompt") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("overview leaked River args: %s", encoded)
	}

	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE id IN ($1,$2)`, oldJobID, extraBacklogID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state='available',attempt=0,finalized_at=NULL,scheduled_at=now() WHERE id IN ($1,$2)`, discardedJobID, extraDiscardedID); err != nil {
		t.Fatal(err)
	}
	cleared, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Alerts == nil || len(cleared.Alerts) != 0 {
		t.Fatalf("cleared alerts = %#v", cleared.Alerts)
	}
}

func TestJobRepositoryRuntimeOverviewProjectsSourceEvidenceAIVaultAndSearchAlertsFromDurableFacts(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	var actorID, sourceID, monitorID, configID, monitorSourceID, runID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO users(email,password_hash,display_name,role,status)
VALUES ('alerts@example.test','fixture-hash','Alerts Admin','admin','active') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO source_connections(source_type,name,endpoint,auth_type,credential_ref,enabled,health_status,created_by,updated_by)
VALUES ('x','alert-source','https://api.x.com/2/tweets/search/recent','bearer','env:X_BEARER_TOKEN',true,'unavailable',$1,$1)
RETURNING id`, actorID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO monitors(name,status,created_by,updated_by) VALUES ('alert-monitor','draft',$1,$1) RETURNING id`, actorID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO monitor_config_versions(monitor_id,revision,state,created_by,updated_by)
VALUES ($1,1,'draft',$2,$2) RETURNING id`, monitorID, actorID).Scan(&configID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO monitor_sources(config_version_id,source_connection_id,enabled)
VALUES ($1,$2,true) RETURNING id`, configID, sourceID).Scan(&monitorSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO source_checkpoints(monitor_source_id,query_hash,next_poll_at,consecutive_failures)
VALUES ($1,$2,now(),3)`, monitorSourceID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO collection_runs(source_connection_id,query_signature,window_start,window_end,trigger_type,scheduled_at,started_at,finished_at,status,error_code)
VALUES ($1,$2,now()-interval '10 minutes',now()-interval '5 minutes','retry',now()-interval '5 minutes',now()-interval '5 minutes',now()-interval '4 minutes','failed','authentication')
RETURNING id`, sourceID, strings.Repeat("b", 64)).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO collection_run_targets(collection_run_id,monitor_source_id,monitor_config_version_id,target_status,error_code,updated_at)
VALUES ($1,$2,$3,'failed','authentication',now()-interval '4 minutes')`, runID, monitorSourceID, configID); err != nil {
		t.Fatal(err)
	}

	var reconciliationRunID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO evidence_lineage_reconciliation_runs(
  scope,status,operator_id,reviewer_id,binary_sha256,schema_sha256,configuration_sha256,
  backup_evidence_sha256,rehearsal_evidence_sha256,batch_size,grace_period_hours,
  examined_count,finding_count,failed_count,failure_code,started_at,completed_at,updated_at)
VALUES ('pg-minio','failed','operator-record','reviewer-record',$1,$1,$1,$1,$1,100,24,1,1,1,
        'object_digest_mismatch',now()-interval '3 minutes',now()-interval '2 minutes',now()-interval '2 minutes')
RETURNING id`, strings.Repeat("c", 64)).Scan(&reconciliationRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO evidence_lineage_reconciliation_items(run_id,scope,asset_type,asset_key_sha256,finding,reason_code)
VALUES ($1,'pg-minio','evidence_snapshot',$2,'digest_mismatch','object_digest_mismatch')`, reconciliationRunID, strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}

	var modelProfileID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO ai_model_profiles(name,task_type,provider,model_name,credential_ref,model_version,max_cost)
VALUES ('alert-profile','event_summary','openai','fixture-model','env:OPENAI_API_KEY','fixture-v1',1)
RETURNING id`).Scan(&modelProfileID); err != nil {
		t.Fatal(err)
	}
	var oldestAIRunID int64
	for index, marker := range []string{"e", "f", "1"} {
		var aiRunID int64
		if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO ai_runs(
  workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,
  model_profile_id,prompt_version,schema_version,input_hash,structured_result,status,started_at,finished_at,
  model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
  error_code,budget_day)
VALUES ('default','event.summary.v1','event_summary','event',$1,1,'agent-http-v1',$2,'prompt-v1','schema-v1',$3,
        '{"secret":"must-not-leak"}'::jsonb,'failed',now()-make_interval(mins => $4),now()-make_interval(mins => $4),
        1,'fixture-v1','parameters-v1','input-v1',$5,$6,70005,(now() AT TIME ZONE 'UTC')::date)
RETURNING id`, int64(100+index), modelProfileID, strings.Repeat(marker, 64), 3-index,
			strings.Repeat(string(rune('2'+index)), 64), strings.Repeat(string(rune('5'+index)), 64)).Scan(&aiRunID); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			oldestAIRunID = aiRunID
		}
	}

	var vaultSyncRunID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO vault_sync_runs(run_type,started_at,finished_at,status,scanned_count,changed_count,conflict_count,error)
VALUES ('reconcile',now()-interval '2 minutes',now()-interval '1 minute','succeeded',4,0,2,'must-not-leak')
RETURNING id`).Scan(&vaultSyncRunID); err != nil {
		t.Fatal(err)
	}

	searchJobID, _, err := queue.NewStore(runtime).Enqueue(ctx, queue.Job{
		Kind: queue.KindGenerateSourceDocument, UniqueKey: "overview-search-backlog",
		DurableArgs: json.RawMessage(`{"entity_id":51,"secret":"must-not-leak"}`),
		ScheduledAt: time.Now().UTC().Add(-6 * time.Minute), MaxAttempts: 2, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	overview, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.AlertPolicyVersion != "p0-operational-alerts-v1" {
		t.Fatalf("alert policy version = %q", overview.AlertPolicyVersion)
	}
	alerts := make(map[string]operationsdomain.RuntimeAlert, len(overview.Alerts))
	for _, alert := range overview.Alerts {
		alerts[alert.AlertID] = alert
		if alert.PolicyVersion != overview.AlertPolicyVersion || alert.Owner != "hotkey-oncall" ||
			alert.RunbookURL == "" || alert.SilenceKey != alert.AlertID || alert.AffectedCount <= 0 || alert.TriggeredAt.IsZero() {
			t.Fatalf("incomplete alert metadata = %#v", alert)
		}
	}
	assertRuntimeAlert := func(alertID, resourceType string, resourceID, jobID, thresholdCount, thresholdSeconds int64) {
		t.Helper()
		alert, found := alerts[alertID]
		if !found || alert.ResourceType != resourceType || alert.ResourceID != resourceID || alert.JobID != jobID ||
			alert.ThresholdCount != thresholdCount || alert.ThresholdSeconds != thresholdSeconds {
			t.Fatalf("%s alert = %#v", alertID, alert)
		}
	}
	assertRuntimeAlert("ALERT-SOURCE-AUTH", "source_connection", sourceID, 0, 3, 0)
	assertRuntimeAlert("ALERT-MINIO-WRITE", "evidence_reconciliation", reconciliationRunID, 0, 1, 0)
	assertRuntimeAlert("ALERT-CODEX-FAILURE", "ai_run", oldestAIRunID, 0, 3, 0)
	assertRuntimeAlert("ALERT-VAULT-CONFLICT", "vault_sync_run", vaultSyncRunID, 0, 1, 0)
	assertRuntimeAlert("ALERT-SEARCH-BACKLOG", "river_job", searchJobID, searchJobID, 1, 300)
	encoded, err := json.Marshal(overview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-leak") || strings.Contains(string(encoded), "example.test") || strings.Contains(string(encoded), "X_BEARER_TOKEN") {
		t.Fatalf("runtime alerts leaked private facts: %s", encoded)
	}

	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE source_checkpoints SET consecutive_failures=0 WHERE monitor_source_id=$1`, monitorSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO evidence_lineage_reconciliation_runs(
  scope,status,operator_id,reviewer_id,binary_sha256,schema_sha256,configuration_sha256,
  backup_evidence_sha256,rehearsal_evidence_sha256,batch_size,grace_period_hours,
  examined_count,healthy_count,started_at,completed_at,updated_at)
VALUES ('pg-minio','completed','operator-record','reviewer-record',$1,$1,$1,$1,$1,100,24,1,1,now(),now(),now())`, strings.Repeat("9", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO ai_runs(
  workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,
  model_profile_id,prompt_version,schema_version,input_hash,status,started_at,finished_at,
  model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
  budget_day)
VALUES ('default','event.summary.v1','event_summary','event',200,1,'agent-http-v1',$1,'prompt-v1','schema-v1',$2,
        'succeeded',now(),now(),1,'fixture-v1','parameters-v1','input-v1',$3,$4,(now() AT TIME ZONE 'UTC')::date)`,
		modelProfileID, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO vault_sync_runs(run_type,started_at,finished_at,status,scanned_count,changed_count,conflict_count)
VALUES ('reconcile',now(),now(),'succeeded',4,0,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE id=$1`, searchJobID); err != nil {
		t.Fatal(err)
	}

	cleared, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, alert := range cleared.Alerts {
		switch alert.AlertID {
		case "ALERT-SOURCE-AUTH", "ALERT-MINIO-WRITE", "ALERT-CODEX-FAILURE", "ALERT-VAULT-CONFLICT", "ALERT-SEARCH-BACKLOG":
			t.Fatalf("alert did not clear after authoritative recovery fact: %#v", alert)
		}
	}
}
