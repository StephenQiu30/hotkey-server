package postgres_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
