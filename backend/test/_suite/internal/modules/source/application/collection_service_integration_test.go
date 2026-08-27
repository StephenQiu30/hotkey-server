package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/jobs"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
)

func TestCollectionServiceFetchesOnceAndDurablyReconcilesEveryTarget(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "shared-capture", 2)
	connector := &collectionConnectorFake{result: domain.FetchResult{
		Items: []domain.SourceItem{{
			SourceCode: "rss", ExternalID: "post-42", ContentType: "article", Title: "Climate safe title",
			Body: "body retained from the source Feed", ObservedAt: time.Date(2026, time.July, 16, 8, 5, 0, 0, time.UTC),
			Metrics: domain.SourceMetrics{ViewCount: domain.KnownMetric(12), CommentCount: domain.KnownMetric(3)},
		}}, NextCursor: "cursor-42", ETag: "etag-42", LastModified: "Wed, 16 Jul 2026 08:05:00 GMT",
	}}
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: connector},
	})
	if err != nil {
		t.Fatalf("NewCollectionService() error = %v", err)
	}

	first, err := service.Collect(context.Background(), request)
	if err != nil {
		t.Fatalf("Collect(first) error = %v", err)
	}
	second, err := service.Collect(context.Background(), request)
	if err != nil {
		t.Fatalf("Collect(second) error = %v", err)
	}
	if first.ID == 0 || second.ID != first.ID || first.Status != domain.CollectionRunSucceeded || second.Status != domain.CollectionRunSucceeded {
		t.Fatalf("collected runs = %#v / %#v, want one succeeded run", first, second)
	}
	if connector.calls.Load() != 1 {
		t.Fatalf("connector Fetch calls = %d, want one shared request", connector.calls.Load())
	}

	var items, reconciled, succeededTargets int
	var payload string
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id = $1`, first.ID).Scan(&items); err != nil {
		t.Fatalf("count captured items: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_target_items WHERE collection_run_id = $1`, first.ID).Scan(&reconciled); err != nil {
		t.Fatalf("count target item reconciliation: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_targets WHERE collection_run_id = $1 AND target_status = 'succeeded'`, first.ID).Scan(&succeededTargets); err != nil {
		t.Fatalf("count succeeded targets: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT captured_item::text FROM collection_run_items WHERE run_id = $1`, first.ID).Scan(&payload); err != nil {
		t.Fatalf("read captured item: %v", err)
	}
	if items != 1 || reconciled != len(request.Targets) || succeededTargets != len(request.Targets) {
		t.Fatalf("items/reconciled/succeeded targets = %d/%d/%d, want 1/%d/%d", items, reconciled, succeededTargets, len(request.Targets), len(request.Targets))
	}
	if strings.Contains(payload, "authorization") || strings.Contains(payload, "never-persist") || strings.Contains(payload, "body retained from the source Feed") || !strings.Contains(payload, `"evidence_completeness": "metadata_only"`) {
		t.Fatalf("captured payload leaked transient or disallowed fields: %s", payload)
	}
	for _, target := range request.Targets {
		var cursor, etag string
		var lastRun int64
		if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, ''), COALESCE(etag, ''), COALESCE(last_successful_run_id, 0) FROM source_checkpoints WHERE monitor_source_id = $1`, target.MonitorSourceID).Scan(&cursor, &etag, &lastRun); err != nil {
			t.Fatalf("read target checkpoint: %v", err)
		}
		if cursor != "cursor-42" || etag != "etag-42" || lastRun != first.ID {
			t.Fatalf("checkpoint = cursor=%q etag=%q run=%d, want successful persisted collection state", cursor, etag, lastRun)
		}
	}
}

func TestCollectionServiceProjectsThreeSourcePartialSuccessWithoutPersistingAggregateOutcome(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	monitorID, requests := collectionRequestsForThreeSourceScan(t, runtime, "three-source-partial")
	retryAfter := requests[2].WindowEnd.Add(15 * time.Minute)
	rss := &collectionConnectorFake{result: domain.FetchResult{Items: []domain.SourceItem{{
		SourceCode: "rss", ExternalID: "rss-result", ContentType: "article", Title: "Climate result continues downstream",
		URL: "https://feeds.example.test/items/rss-result", ObservedAt: requests[0].WindowStart.Add(time.Minute),
	}}, NextCursor: "rss-cursor"}}
	hackerNews := &collectionConnectorFake{result: domain.FetchResult{NextCursor: "hn-empty-cursor"}}
	x := &collectionConnectorFake{
		result: domain.FetchResult{RateLimit: domain.RateLimit{RetryAfter: &retryAfter}},
		err:    domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("synthetic upstream detail")),
	}
	registry := collectionConnectorRegistryByTypeFake{connectors: map[domain.SourceType]domain.Connector{
		domain.SourceTypeRSS: rss, domain.SourceTypeHackerNews: hackerNews, domain.SourceTypeX: x,
	}}
	runs := sourcepostgres.NewCollectionRepository(runtime)
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs,
		Connectors: registry, Now: func() time.Time { return requests[0].WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(): %v", err)
	}
	jobs := queue.NewStore(runtime)
	collected := make([]domain.CollectionRun, 0, len(requests))
	for index, request := range requests {
		request := request
		run, collectErr := service.CollectWithSuccessHook(context.Background(), request, func(ctx context.Context, runID int64) error {
			_, _, enqueueErr := jobs.Enqueue(ctx, queue.Job{
				Kind: queue.KindNormalizeContent, UniqueKey: queue.StableJobKey(queue.KindNormalizeContent, runID, 1, request.QuerySignature),
				Payload:     queue.Payload{EntityID: runID, EntityVersion: 1, WindowStart: request.WindowStart, WindowEnd: request.WindowEnd, InputHash: request.QuerySignature},
				ScheduledAt: request.ScheduledAt, MaxAttempts: 3, Priority: 2,
			})
			return enqueueErr
		})
		collected = append(collected, run)
		if index < 2 && (collectErr != nil || run.Status != domain.CollectionRunSucceeded) {
			t.Fatalf("Collect(success source %d) run/error = %#v / %v", index, run, collectErr)
		}
		if index == 2 && (domain.ClassifyCollectionError(collectErr) != domain.CollectionErrorRateLimited || run.Status != domain.CollectionRunFailed) {
			t.Fatalf("Collect(rate-limited X) run/error = %#v / %v", run, collectErr)
		}
	}

	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Connectors: registry,
		Retries: collectionRetryActivatorFake{}, Scans: monitorpostgres.NewMonitorScanReader(runtime),
	})
	if err != nil {
		t.Fatalf("NewCollectionControlService(): %v", err)
	}
	scans, err := control.Scans(context.Background(), sourceapplication.MonitorScanListInput{
		Subject: identitydomain.Subject{UserID: 7, SessionID: 9, Role: identitydomain.RoleViewer}, MonitorID: monitorID, Limit: 10,
	})
	if err != nil || len(scans) != 1 {
		t.Fatalf("Scans() scans/error = %#v / %v, want one aggregate scan", scans, err)
	}
	scan := scans[0]
	if scan.Status != domain.MonitorScanPartial || scan.RunOutcome != domain.MonitorScanOutcomePartialSuccess || len(scan.Sources) != 3 {
		t.Fatalf("aggregate scan = %#v, want three-source partial_success projection", scan)
	}
	if scan.CandidateCount != 1 || scan.AcceptedCount != 1 || scan.RejectedCount != 0 {
		t.Fatalf("aggregate counts = candidate:%d accepted:%d rejected:%d", scan.CandidateCount, scan.AcceptedCount, scan.RejectedCount)
	}
	wantSources := []struct {
		sourceType domain.SourceType
		status     domain.CollectionRunStatus
		accepted   int64
		errorCode  string
	}{
		{sourceType: domain.SourceTypeRSS, status: domain.CollectionRunSucceeded, accepted: 1},
		{sourceType: domain.SourceTypeHackerNews, status: domain.CollectionRunSucceeded},
		{sourceType: domain.SourceTypeX, status: domain.CollectionRunFailed, errorCode: string(domain.CollectionErrorRateLimited)},
	}
	for index, want := range wantSources {
		got := scan.Sources[index]
		if got.SourceType != string(want.sourceType) || got.Status != want.status || got.AcceptedCount != want.accepted || got.ErrorCode != want.errorCode {
			t.Fatalf("source %d = %#v, want type=%q status=%q accepted=%d error=%q", index, got, want.sourceType, want.status, want.accepted, want.errorCode)
		}
	}
	var normalizeRuns []int64
	rows, err := runtime.SQL.Query(`SELECT (args->>'entity_id')::bigint FROM river_job WHERE kind = $1 ORDER BY id`, queue.KindNormalizeContent)
	if err != nil {
		t.Fatalf("query normalize jobs: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			t.Fatalf("scan normalize job: %v", err)
		}
		normalizeRuns = append(normalizeRuns, runID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read normalize jobs: %v", err)
	}
	if len(normalizeRuns) != 2 || normalizeRuns[0] != collected[0].ID || normalizeRuns[1] != collected[1].ID {
		t.Fatalf("normalize runs = %v, want only successful RSS/HN runs %d/%d", normalizeRuns, collected[0].ID, collected[1].ID)
	}
	if _, err := runtime.SQL.Exec(`UPDATE collection_runs SET status = 'partial_success' WHERE id = $1`, collected[0].ID); err == nil {
		t.Fatal("collection_runs unexpectedly persisted derived partial_success outcome")
	}
}

func TestCollectionServiceDistinguishesPartialItemResultsFromFullFailure(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()

	partialRequest := collectionRequestForService(t, runtime, "partial-item-window", 1)
	partialConnector := &collectionConnectorFake{result: domain.FetchResult{
		Items: []domain.SourceItem{{
			SourceCode: "hacker_news", ExternalID: "101", ContentType: "article", Title: "Climate completed prefix",
			URL: "https://news.ycombinator.com/item?id=101", ObservedAt: partialRequest.WindowStart,
		}},
		NextCursor: "101", HasMore: true,
		Diagnostics: []domain.FetchDiagnostic{{Code: "item_temporary_failure", SourceExternalID: "102"}},
	}}
	partialService, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: partialConnector}, Now: func() time.Time { return partialRequest.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(partial): %v", err)
	}
	partialRun, err := partialService.Collect(context.Background(), partialRequest)
	if err != nil || partialRun.Status != domain.CollectionRunSucceeded {
		t.Fatalf("Collect(partial) run/error = %#v / %v, want succeeded run", partialRun, err)
	}
	var partialStatus, partialCursor string
	var partialAccepted, partialRejected int64
	if err := runtime.SQL.QueryRow(`
SELECT status, COALESCE(next_cursor, ''), accepted_count, rejected_count
FROM collection_runs WHERE id = $1`, partialRun.ID).Scan(&partialStatus, &partialCursor, &partialAccepted, &partialRejected); err != nil {
		t.Fatalf("read partial run: %v", err)
	}
	if partialStatus != "succeeded" || partialCursor != "101" || partialAccepted != 1 || partialRejected != 1 {
		t.Fatalf("partial run facts = %q cursor=%q accepted=%d rejected=%d", partialStatus, partialCursor, partialAccepted, partialRejected)
	}

	failedRequest := collectionRequestForService(t, runtime, "full-item-window-failure", 1)
	failedConnector := &collectionConnectorFake{err: domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("all item requests failed"))}
	failedService, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: failedConnector}, Now: func() time.Time { return failedRequest.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(failed): %v", err)
	}
	failedRun, err := failedService.Collect(context.Background(), failedRequest)
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || failedRun.Status != domain.CollectionRunFailed {
		t.Fatalf("Collect(failed) run/error = %#v / %v, want failed temporary run", failedRun, err)
	}
}

func TestManualCollectionUsesDurableCooldownAndActivePublishedTargets(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "manual-cooldown", 1)
	target := request.Targets[0]
	var monitorID int64
	if err := runtime.SQL.QueryRow(`SELECT monitor_id FROM monitor_config_versions WHERE id = $1`, target.MonitorConfigVersionID).Scan(&monitorID); err != nil {
		t.Fatalf("read monitor id: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, origin, approval_status)
VALUES ($1, 'keyword', 'contains', 'climate', 'user', 'approved')`, target.MonitorConfigVersionID); err != nil {
		t.Fatalf("create manual monitor rule: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = now() WHERE id = $2`, strings.Repeat("d", 64), target.MonitorConfigVersionID); err != nil {
		t.Fatalf("publish monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status = 'active', published_config_version_id = $1 WHERE id = $2`, target.MonitorConfigVersionID, monitorID); err != nil {
		t.Fatalf("activate monitor: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('d',64),ready_at=now() WHERE id=$1`, target.CompiledProfileID); err != nil {
		t.Fatalf("ready manual compiled profile: %v", err)
	}

	store := queue.NewStore(runtime)
	manuals, err := sourcejobs.NewManualCollectionActivator(store)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 16, 9, 2, 0, 0, time.UTC)
	targetReader := monitorpostgres.NewPublishedCollectionTargetReader(runtime)
	monitorAuthorizer := &collectionMonitorAuthorizerFake{allowedUserID: 77, allowedMonitorID: monitorID}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}}, Retries: collectionRetryActivatorFake{},
		Manuals: manuals, Targets: targetReader, Quota: operationspostgres.NewGovernanceRepository(runtime),
		Monitors: monitorAuthorizer, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	var editorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role) VALUES ('manual-quota@example.test','hash','Manual quota','editor') RETURNING id`).Scan(&editorID); err != nil {
		t.Fatal(err)
	}
	editor := identitydomain.Subject{UserID: editorID, SessionID: 2, Role: identitydomain.RoleEditor}
	first, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: editor, MonitorID: monitorID})
	if err != nil || first.Requested != 1 || first.Created != 1 || first.Reused != 0 || !first.CooldownUntil.Equal(time.Date(2026, time.July, 16, 9, 5, 0, 0, time.UTC)) {
		t.Fatalf("Manual(first) = %#v / %v", first, err)
	}
	second, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: editor, MonitorID: monitorID})
	if err != nil || second.Created != 0 || second.Reused != 1 {
		t.Fatalf("Manual(second) = %#v / %v, want cooldown reuse", second, err)
	}
	analyst := identitydomain.Subject{UserID: 77, SessionID: 3, Role: identitydomain.RoleAnalyst}
	analystResult, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: analyst, MonitorID: monitorID})
	if err != nil || analystResult.Created != 0 || analystResult.Reused != 1 {
		t.Fatalf("Manual(authorized analyst) = %#v / %v, want owner-authorized cooldown reuse", analystResult, err)
	}
	otherAnalyst := identitydomain.Subject{UserID: 78, SessionID: 4, Role: identitydomain.RoleAnalyst}
	if _, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: otherAnalyst, MonitorID: monitorID}); err == nil {
		t.Fatal("Manual(non-owner analyst) unexpectedly succeeded")
	}
	if monitorAuthorizer.calls != 2 {
		t.Fatalf("monitor contribution authorization calls = %d, want 2 analyst calls", monitorAuthorizer.calls)
	}
	var jobCount int
	var triggerType string
	if err := runtime.SQL.QueryRow(`SELECT count(*), min(args->>'trigger_type') FROM river_job WHERE kind = 'collect_source'`).Scan(&jobCount, &triggerType); err != nil {
		t.Fatalf("read manual jobs: %v", err)
	}
	if jobCount != 1 || triggerType != "manual" {
		t.Fatalf("manual jobs = %d trigger=%q, want one manual envelope", jobCount, triggerType)
	}
	collections, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{result: domain.FetchResult{Items: []domain.SourceItem{{
			SourceCode: "rss", ExternalID: "manual-item", ContentType: "article", Title: "Manual collection item", ObservedAt: now,
		}}}}}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := sourcejobs.NewCollectHandler(collections, targetReader, store)
	if err != nil {
		t.Fatal(err)
	}
	manualWindowStart := now.Add(-target.CollectionInterval)
	manualRequest := request
	manualRequest.WindowStart, manualRequest.WindowEnd = manualWindowStart, now
	err = handler.Handle(context.Background(), collectionQueueJob(t, manualRequest, now, domain.CollectionTriggerManual))
	if err != nil {
		t.Fatalf("manual collect handler: %v", err)
	}
	var runTrigger, runStatus string
	var normalizeJobs int
	if err := runtime.SQL.QueryRow(`SELECT trigger_type, status FROM collection_runs ORDER BY id DESC LIMIT 1`).Scan(&runTrigger, &runStatus); err != nil {
		t.Fatalf("read manual collection run: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM river_job WHERE kind = 'normalize_content'`).Scan(&normalizeJobs); err != nil {
		t.Fatalf("count manual normalize jobs: %v", err)
	}
	if runTrigger != "manual" || runStatus != "succeeded" || normalizeJobs != 1 {
		t.Fatalf("manual pipeline = trigger %q status %q normalize jobs %d", runTrigger, runStatus, normalizeJobs)
	}

	now = now.Add(5 * time.Minute)
	third, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: editor, MonitorID: monitorID})
	if err != nil || third.Created != 1 || third.Reused != 0 {
		t.Fatalf("Manual(next bucket) = %#v / %v", third, err)
	}
	var consumed int64
	if err := runtime.SQL.QueryRow(`SELECT used FROM quota_usage_ledgers WHERE subject_id=$1`, editorID).Scan(&consumed); err != nil || consumed != 2 {
		t.Fatalf("manual search usage = %d/%v, want 2", consumed, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status = 'paused' WHERE id = $1`, monitorID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	if _, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: editor, MonitorID: monitorID}); err == nil {
		t.Fatal("Manual(paused monitor) unexpectedly succeeded")
	}
	viewer := identitydomain.Subject{UserID: 3, SessionID: 3, Role: identitydomain.RoleViewer}
	if _, err := control.Manual(context.Background(), sourceapplication.ManualCollectionInput{Subject: viewer, MonitorID: monitorID}); err == nil {
		t.Fatal("Manual(viewer) unexpectedly succeeded")
	}
}

func TestCollectionServiceFailureRetainsCursorAndPersistsRetryState(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "rate-limit", 1)
	request.Targets[0].Checkpoint.CursorValue = "durable-cursor"
	if _, err := runtime.SQL.Exec(`UPDATE source_checkpoints SET cursor_value = $1 WHERE id = $2`, request.Targets[0].Checkpoint.CursorValue, request.Targets[0].Checkpoint.ID); err != nil {
		t.Fatalf("seed checkpoint cursor: %v", err)
	}
	now := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	retryAfter := now.Add(15 * time.Minute)
	connector := &collectionConnectorFake{
		result: domain.FetchResult{RateLimit: domain.RateLimit{RetryAfter: &retryAfter}},
		err:    domain.NewCollectionError(domain.CollectionErrorRateLimited, fmt.Errorf("limited")),
	}
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: connector}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewCollectionService() error = %v", err)
	}
	run, err := service.Collect(context.Background(), request)
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || run.Status != domain.CollectionRunFailed {
		t.Fatalf("Collect(rate limited) run/error = %#v / %v, want failed rate-limited run", run, err)
	}
	var cursor string
	var failures int
	var nextPollAt time.Time
	var targetStatus, runStatus string
	var persistedRetry time.Time
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, ''), consecutive_failures, next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&cursor, &failures, &nextPollAt); err != nil {
		t.Fatalf("read failure checkpoint: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT status, retry_after FROM collection_runs WHERE id = $1`, run.ID).Scan(&runStatus, &persistedRetry); err != nil {
		t.Fatalf("read failed run: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1`, run.ID).Scan(&targetStatus); err != nil {
		t.Fatalf("read failed target: %v", err)
	}
	if cursor != "durable-cursor" || failures != 1 || !nextPollAt.Equal(retryAfter) || !persistedRetry.Equal(retryAfter) || runStatus != "failed" || targetStatus != "failed" {
		t.Fatalf("failure persistence = cursor=%q failures=%d next=%s retry=%s run=%q target=%q", cursor, failures, nextPollAt, persistedRetry, runStatus, targetStatus)
	}
}

func TestCollectionWorkerDefersRateLimitAndReplaysFailedWindowOnceDue(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "automatic-rate-limit-retry", 1)
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_rules (config_version_id,rule_type,operator,value,origin,approval_status)
VALUES ($1,'keyword','contains','climate','user','approved')`, request.Targets[0].MonitorConfigVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_config_versions SET state='published',published_at=now(),config_hash=repeat('a',64)
WHERE id=$1`, request.Targets[0].MonitorConfigVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitors SET status='active',published_config_version_id=$1
WHERE id=(SELECT monitor_id FROM monitor_config_versions WHERE id=$1)`, request.Targets[0].MonitorConfigVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('d',64),ready_at=now() WHERE id=$1`, request.Targets[0].CompiledProfileID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	resetAt := now.Add(15 * time.Minute)
	connector := &collectionConnectorFake{
		results: []domain.FetchResult{{RateLimit: domain.RateLimit{RetryAfter: &resetAt}}, {NextCursor: "recovered-cursor"}},
		errors:  []error{domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("provider detail")), nil},
	}
	store := queue.NewStore(runtime)
	job := collectionQueueJob(t, request, now.Add(-time.Minute), domain.CollectionTriggerSchedule)
	jobID, _, err := store.Enqueue(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: connector}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := sourcejobs.NewCollectHandler(service, monitorpostgres.NewPublishedCollectionTargetReader(runtime), store)
	if err != nil {
		t.Fatal(err)
	}
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindCollectSource: handler.Handle})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(rate limit) = %t/%v", worked, err)
	}
	var runID int64
	var runStatus, jobState string
	var scheduledAt time.Time
	if err := runtime.SQL.QueryRow(`SELECT id,status FROM collection_runs WHERE source_connection_id=$1 AND query_signature=$2`, request.SourceConnectionID, request.QuerySignature).Scan(&runID, &runStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT state,scheduled_at FROM river_job WHERE id=$1`, jobID).Scan(&jobState, &scheduledAt); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" || jobState != "available" || !scheduledAt.Equal(resetAt) || connector.calls.Load() != 1 {
		t.Fatalf("deferred retry facts = run %q job %q at %s calls %d", runStatus, jobState, scheduledAt, connector.calls.Load())
	}

	// Advance the durable reset boundary without sleeping. The same run/window
	// must be claimed and completed; no duplicate collection run is created.
	if _, err := runtime.SQL.Exec(`UPDATE collection_runs SET retry_after=now()-interval '1 second' WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE river_job SET scheduled_at=now()-interval '1 second' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(recovered) = %t/%v", worked, err)
	}
	var runCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*),min(status) FROM collection_runs WHERE source_connection_id=$1 AND query_signature=$2`, request.SourceConnectionID, request.QuerySignature).Scan(&runCount, &runStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT state FROM river_job WHERE id=$1`, jobID).Scan(&jobState); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 || runStatus != "succeeded" || jobState != "completed" || connector.calls.Load() != 2 {
		t.Fatalf("recovered retry facts = runs %d status %q job %q calls %d", runCount, runStatus, jobState, connector.calls.Load())
	}
}

func TestCollectionServicePersistsAuthenticationAndPermanentFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		kind domain.CollectionErrorKind
	}{
		{name: "authentication", kind: domain.CollectionErrorAuthentication},
		{name: "parse", kind: domain.CollectionErrorParse},
		{name: "permanent", kind: domain.CollectionErrorPermanent},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := openRuntime(t)
			defer func() { _ = runtime.Close() }()
			request := collectionRequestForService(t, runtime, test.name, 1)
			request.Targets[0].Checkpoint.CursorValue = "must-stay"
			if _, err := runtime.SQL.Exec(`UPDATE source_checkpoints SET cursor_value = $1 WHERE id = $2`, "must-stay", request.Targets[0].Checkpoint.ID); err != nil {
				t.Fatalf("seed cursor: %v", err)
			}
			connector := &collectionConnectorFake{err: domain.NewCollectionError(test.kind, fmt.Errorf("upstream failure"))}
			service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
				Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
				Connectors: collectionConnectorRegistryFake{connector: connector}, Now: func() time.Time { return request.WindowEnd },
			})
			if err != nil {
				t.Fatalf("NewCollectionService(): %v", err)
			}
			run, err := service.Collect(context.Background(), request)
			if err == nil || domain.ClassifyCollectionError(err) != test.kind || run.Status != domain.CollectionRunFailed {
				t.Fatalf("Collect() run/error = %#v / %v, want failed %q run", run, err, test.kind)
			}
			var cursor, errorCode string
			if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, '') FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&cursor); err != nil {
				t.Fatalf("read cursor: %v", err)
			}
			if err := runtime.SQL.QueryRow(`SELECT COALESCE(error_code, '') FROM collection_runs WHERE id = $1`, run.ID).Scan(&errorCode); err != nil {
				t.Fatalf("read error code: %v", err)
			}
			if cursor != "must-stay" || errorCode != string(test.kind) {
				t.Fatalf("failure persistence = cursor=%q error_code=%q, want retained cursor and %q", cursor, errorCode, test.kind)
			}
			if replayed, replayErr := service.Collect(context.Background(), request); replayErr != nil || replayed.Status != domain.CollectionRunFailed || connector.calls.Load() != 1 {
				t.Fatalf("non-retryable replay = %#v/%v calls=%d, want same failed run without another request", replayed, replayErr, connector.calls.Load())
			}
		})
	}
}

func TestCollectionServiceRestartUsesPersistedCursorAndNoContentKeepsIt(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	firstRequest := collectionRequestForService(t, runtime, "restart", 1)
	now := time.Date(2026, time.July, 16, 10, 0, 0, 0, time.UTC)
	firstConnector := &collectionConnectorFake{result: domain.FetchResult{NextCursor: "persisted-cursor", ETag: "persisted-etag"}}
	firstService, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: firstConnector}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(first) error = %v", err)
	}
	if _, err := firstService.Collect(context.Background(), firstRequest); err != nil {
		t.Fatalf("Collect(first) error = %v", err)
	}

	secondRequest := firstRequest
	secondRequest.WindowStart = firstRequest.WindowEnd
	secondRequest.WindowEnd = secondRequest.WindowStart.Add(time.Hour)
	secondRequest.Targets = append([]domain.PublishedCollectionTarget(nil), firstRequest.Targets...)
	var checkpointID, version int64
	var cursor, etag string
	if err := runtime.SQL.QueryRow(`SELECT id, version, COALESCE(cursor_value, ''), COALESCE(etag, '') FROM source_checkpoints WHERE monitor_source_id = $1`, secondRequest.Targets[0].MonitorSourceID).Scan(&checkpointID, &version, &cursor, &etag); err != nil {
		t.Fatalf("read persisted checkpoint: %v", err)
	}
	secondRequest.Targets[0].Checkpoint.ID = checkpointID
	secondRequest.Targets[0].Checkpoint.Version = version
	secondRequest.Targets[0].Checkpoint.CursorValue = cursor
	secondRequest.Targets[0].Checkpoint.ETag = etag
	secondRequest.Targets[0].Checkpoint.NextPollAt = now
	secondConnector := &collectionConnectorFake{result: domain.FetchResult{}}
	secondService, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: secondConnector}, Now: func() time.Time { return now.Add(time.Hour) },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(restarted) error = %v", err)
	}
	if _, err := secondService.Collect(context.Background(), secondRequest); err != nil {
		t.Fatalf("Collect(restarted no-content) error = %v", err)
	}
	requests := secondConnector.fetchRequests()
	if len(requests) != 1 || requests[0].RequestCursor != "persisted-cursor" || requests[0].ETag != "persisted-etag" {
		t.Fatalf("restart fetch request = %#v, want persisted cursor and validator", requests)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, ''), COALESCE(etag, '') FROM source_checkpoints WHERE id = $1`, checkpointID).Scan(&cursor, &etag); err != nil {
		t.Fatalf("read no-content checkpoint: %v", err)
	}
	if cursor != "persisted-cursor" || etag != "persisted-etag" {
		t.Fatalf("no-content checkpoint = cursor=%q etag=%q, want retained persisted state", cursor, etag)
	}
}

func TestCollectionServiceIsolatesOneTargetCheckpointConflict(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "target-isolation", 2)
	if _, err := runtime.SQL.Exec(`UPDATE source_checkpoints SET version = version + 1 WHERE id = $1`, request.Targets[0].Checkpoint.ID); err != nil {
		t.Fatalf("make first checkpoint stale: %v", err)
	}
	connector := &collectionConnectorFake{result: domain.FetchResult{Items: []domain.SourceItem{{
		SourceCode: "rss", ExternalID: "isolated-item", ContentType: "article", ObservedAt: request.WindowStart,
	}}, NextCursor: "isolated-cursor"}}
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: connector}, Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService() error = %v", err)
	}
	run, err := service.Collect(context.Background(), request)
	if err != nil || run.Status != domain.CollectionRunSucceeded {
		t.Fatalf("Collect() run/error = %#v / %v, want succeeded shared run", run, err)
	}
	var firstStatus, secondStatus, firstCursor, secondCursor string
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1 AND monitor_source_id = $2`, run.ID, request.Targets[0].MonitorSourceID).Scan(&firstStatus); err != nil {
		t.Fatalf("read failed target: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1 AND monitor_source_id = $2`, run.ID, request.Targets[1].MonitorSourceID).Scan(&secondStatus); err != nil {
		t.Fatalf("read succeeded target: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, '') FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&firstCursor); err != nil {
		t.Fatalf("read stale checkpoint: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, '') FROM source_checkpoints WHERE id = $1`, request.Targets[1].Checkpoint.ID).Scan(&secondCursor); err != nil {
		t.Fatalf("read successful checkpoint: %v", err)
	}
	if firstStatus != "failed" || secondStatus != "succeeded" || firstCursor != "" || secondCursor != "isolated-cursor" {
		t.Fatalf("target isolation = first=%q/%q second=%q/%q", firstStatus, firstCursor, secondStatus, secondCursor)
	}
}

func TestCollectionControlListsRetriesAndPersistsSafeHealth(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "admin-control", 1)
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateOrReuseRun(): %v", err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun() started/error = %t / %v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatalf("PersistFailure(): %v", err)
	}
	checkedAt := time.Date(2026, time.July, 16, 13, 0, 0, 0, time.UTC)
	metrics := &collectionMetricsFake{}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{health: domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorTemporary, DiagnosticCode: "request_failed"}}},
		Metrics:    metrics, Retries: collectionRetryActivatorFake{}, Now: func() time.Time { return checkedAt },
	})
	if err != nil {
		t.Fatalf("NewCollectionControlService(): %v", err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	page, err := control.List(context.Background(), sourceapplication.CollectionRunListInput{Subject: admin, Query: domain.CollectionRunListQuery{Limit: 10}})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != run.ID || page.Items[0].Status != domain.CollectionRunFailed {
		t.Fatalf("List() page/error = %#v / %v, want failed run summary", page, err)
	}
	retried, err := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: run.ID})
	if err != nil || retried.Status != domain.CollectionRunQueued || len(retried.Targets) != 1 || retried.Targets[0].Status != domain.CollectionRunQueued {
		t.Fatalf("Retry() summary/error = %#v / %v, want queued run and target", retried, err)
	}
	health, err := control.Health(context.Background(), sourceapplication.SourceHealthInput{Subject: admin, ID: request.SourceConnectionID})
	if err != nil || health.Healthy || !health.CheckedAt.Equal(checkedAt) || health.ErrorCode != "request_failed" {
		t.Fatalf("Health() result/error = %#v / %v, want safe unhealthy temporary result", health, err)
	}
	var status string
	if err := runtime.SQL.QueryRow(`SELECT health_status FROM source_connections WHERE id = $1`, request.SourceConnectionID).Scan(&status); err != nil {
		t.Fatalf("read persisted source health: %v", err)
	}
	if status != string(domain.HealthStatusDegraded) {
		t.Fatalf("persisted health status = %q, want %q", status, domain.HealthStatusDegraded)
	}
	if !metrics.recorded("list", "success") || !metrics.recorded("retry", "success") || !metrics.recorded("health", "unhealthy") {
		t.Fatalf("collection metrics = %#v, want list/retry/health observations", metrics.values)
	}
}

func TestCollectionControlRetryAtomicallyRestoresCheckpointAndJob(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "atomic-retry", 1)
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, origin, approval_status)
VALUES ($1, 'keyword', 'contains', 'climate', 'user', 'approved')`, request.Targets[0].MonitorConfigVersionID); err != nil {
		t.Fatalf("create monitor rule: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_config_versions SET state = 'published', published_at = now(), config_hash = repeat('a', 64)
WHERE id = $1`, request.Targets[0].MonitorConfigVersionID); err != nil {
		t.Fatalf("publish config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitors SET status = 'active', published_config_version_id = $1
WHERE id = (SELECT monitor_id FROM monitor_config_versions WHERE id = $1)`, request.Targets[0].MonitorConfigVersionID); err != nil {
		t.Fatalf("activate monitor: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('d',64),ready_at=now() WHERE id=$1`, request.Targets[0].CompiledProfileID); err != nil {
		t.Fatalf("ready compiled profile: %v", err)
	}
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd}); err != nil {
		t.Fatal(err)
	}
	store := queue.NewStore(runtime)
	job := collectionQueueJob(t, request, request.WindowStart, domain.CollectionTriggerSchedule)
	jobID, _, err := store.Enqueue(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE river_job SET state = 'discarded', attempt = 1, finalized_at = now() WHERE id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	targetReader := monitorpostgres.NewPublishedCollectionTargetReader(runtime)
	activator, err := sourcejobs.NewCollectionRetryActivator(targetReader, store)
	if err != nil {
		t.Fatal(err)
	}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: activator,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	retried, err := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: run.ID})
	if err != nil || retried.Status != domain.CollectionRunQueued {
		t.Fatalf("Retry() = %#v/%v", retried, err)
	}
	var checkpoint time.Time
	var jobState string
	var attempt, maxAttempts int
	if err := runtime.SQL.QueryRow(`SELECT next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT state, attempt, max_attempts FROM river_job WHERE id = $1`, jobID).Scan(&jobState, &attempt, &maxAttempts); err != nil {
		t.Fatal(err)
	}
	if !checkpoint.Equal(request.WindowStart) || jobState != "available" || attempt != 1 || maxAttempts != 4 {
		t.Fatalf("retry facts = checkpoint %s job %s attempt %d/%d", checkpoint, jobState, attempt, maxAttempts)
	}
	collections, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}}, Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := sourcejobs.NewCollectHandler(collections, targetReader, store)
	if err != nil {
		t.Fatal(err)
	}
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindCollectSource: handler.Handle})
	worked, err := worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce() = %t/%v", worked, err)
	}
	var runStatus string
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, run.ID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != string(domain.CollectionRunSucceeded) {
		var state, attemptError string
		_ = runtime.SQL.QueryRow(`SELECT state FROM river_job WHERE id = $1`, jobID).Scan(&state)
		_ = runtime.SQL.QueryRow(`SELECT COALESCE(error, '') FROM river_job_attempt WHERE job_id = $1 ORDER BY attempt DESC LIMIT 1`, jobID).Scan(&attemptError)
		t.Fatalf("run status = %q, job=%q error=%q", runStatus, state, attemptError)
	}
}

func TestCollectionControlRetryRejectsIncompleteEligibleTargetSet(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "incomplete-target-set", 2)
	for _, target := range request.Targets {
		if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, origin, approval_status)
VALUES ($1, 'keyword', 'contains', 'climate', 'user', 'approved')`, target.MonitorConfigVersionID); err != nil {
			t.Fatalf("create monitor rule: %v", err)
		}
		if _, err := runtime.SQL.Exec(`
UPDATE monitor_config_versions SET state = 'published', published_at = now(), config_hash = repeat('a', 64)
WHERE id = $1`, target.MonitorConfigVersionID); err != nil {
			t.Fatalf("publish config: %v", err)
		}
		if _, err := runtime.SQL.Exec(`
UPDATE monitors SET status = 'active', published_config_version_id = $1
WHERE id = (SELECT monitor_id FROM monitor_config_versions WHERE id = $1)`, target.MonitorConfigVersionID); err != nil {
			t.Fatalf("activate monitor: %v", err)
		}
		if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('d',64),ready_at=now() WHERE id=$1`, target.CompiledProfileID); err != nil {
			t.Fatalf("ready compiled profile: %v", err)
		}
	}
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatal(err)
	}
	store := queue.NewStore(runtime)
	jobID, _, err := store.Enqueue(context.Background(), collectionQueueJob(t, request, request.WindowStart, domain.CollectionTriggerSchedule))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE river_job SET state = 'discarded', attempt = 1, finalized_at = now() WHERE id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitors SET status = 'paused'
WHERE id = (SELECT monitor_id FROM monitor_config_versions WHERE id = $1)`, request.Targets[1].MonitorConfigVersionID); err != nil {
		t.Fatal(err)
	}
	var checkpointBefore time.Time
	if err := runtime.SQL.QueryRow(`SELECT next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&checkpointBefore); err != nil {
		t.Fatal(err)
	}
	targetReader := monitorpostgres.NewPublishedCollectionTargetReader(runtime)
	activator, err := sourcejobs.NewCollectionRetryActivator(targetReader, store)
	if err != nil {
		t.Fatal(err)
	}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: activator,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	if _, err := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: run.ID}); err == nil {
		t.Fatal("Retry() unexpectedly accepted an incomplete eligible target set")
	}
	var runStatus, firstTargetStatus, secondTargetStatus, jobState string
	var checkpointAfter time.Time
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, run.ID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1 AND monitor_source_id = $2`, run.ID, request.Targets[0].MonitorSourceID).Scan(&firstTargetStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1 AND monitor_source_id = $2`, run.ID, request.Targets[1].MonitorSourceID).Scan(&secondTargetStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&checkpointAfter); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT state FROM river_job WHERE id = $1`, jobID).Scan(&jobState); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" || firstTargetStatus != "failed" || secondTargetStatus != "failed" || !checkpointAfter.Equal(checkpointBefore) || jobState != "discarded" {
		t.Fatalf("rollback facts = run=%q targets=%q/%q checkpoint=%s job=%q", runStatus, firstTargetStatus, secondTargetStatus, checkpointAfter, jobState)
	}
}

func TestCollectionControlRetryRejectsAdvancedCheckpointAndRollsBack(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "advanced-checkpoint", 1)
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE source_checkpoints SET cursor_value = 'newer-cursor', version = version + 1 WHERE id = $1`, request.Targets[0].Checkpoint.ID); err != nil {
		t.Fatal(err)
	}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: collectionRetryActivatorFake{},
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	if _, err := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: run.ID}); err == nil {
		t.Fatal("Retry() unexpectedly accepted an advanced checkpoint")
	}
	var runStatus, targetStatus, cursor string
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, run.ID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1`, run.ID).Scan(&targetStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, '') FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" || targetStatus != "failed" || cursor != "newer-cursor" {
		t.Fatalf("rollback facts = run=%q target=%q cursor=%q", runStatus, targetStatus, cursor)
	}
}

func TestCollectionControlRetryIsConcurrentSafe(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "concurrent-retry", 1)
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatal(err)
	}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: collectionRetryActivatorFake{},
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, retryErr := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: run.ID})
			results <- retryErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for retryErr := range results {
		if retryErr == nil {
			succeeded++
		} else {
			conflicted++
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent retry results = success %d conflict %d", succeeded, conflicted)
	}
}

func TestCollectionClaimSerializesRetryAndNewerWorker(t *testing.T) {
	t.Run("retry wins", testCollectionClaimRetryWins)
	t.Run("newer worker wins", testCollectionClaimNewerWorkerWins)
}

func testCollectionClaimRetryWins(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "retry-worker-claim-retry-wins", 1)
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatal(err)
	}
	newer := newerCollectionRequest(t, runtime, request)
	activator := &blockingCollectionRetryActivator{entered: make(chan struct{}), release: make(chan struct{})}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: activator,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	collections, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	retryResult := make(chan error, 1)
	go func() {
		_, retryErr := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: run.ID})
		retryResult <- retryErr
	}()
	<-activator.entered
	workerAttempted := make(chan struct{})
	retryWon := errors.New("retry restored the old window before worker resolution")
	workerResult := make(chan error, 1)
	go func() {
		close(workerAttempted)
		_, workerErr := collections.CollectResolvedWithSuccessHook(context.Background(), request.SourceConnectionID, request.QuerySignature, func(ctx context.Context) (domain.CollectionRequest, error) {
			transaction, found := database.TransactionFromContext(ctx)
			if !found {
				return domain.CollectionRequest{}, errors.New("worker resolver lost caller transaction")
			}
			var nextPoll time.Time
			if err := transaction.SQL.QueryRowContext(ctx, `SELECT next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&nextPoll); err != nil {
				return domain.CollectionRequest{}, err
			}
			if !nextPoll.Equal(newer.WindowStart) {
				return domain.CollectionRequest{}, retryWon
			}
			return newer, nil
		}, nil)
		workerResult <- workerErr
	}()
	<-workerAttempted
	waitForAdvisoryWaiter(t, runtime)
	close(activator.release)
	if retryErr := <-retryResult; retryErr != nil {
		t.Fatalf("Retry() error = %v", retryErr)
	}
	if workerErr := <-workerResult; !errors.Is(workerErr, retryWon) {
		t.Fatalf("newer worker error = %v, want restored-window signal", workerErr)
	}
	var runCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_runs WHERE source_connection_id = $1 AND query_signature = $2`, request.SourceConnectionID, request.QuerySignature).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("collection run count = %d, want no phantom newer run", runCount)
	}
}

func testCollectionClaimNewerWorkerWins(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "retry-worker-claim-worker-wins", 1)
	runs := sourcepostgres.NewCollectionRepository(runtime)
	failed, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), failed.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: failed.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatal(err)
	}
	newer := newerCollectionRequest(t, runtime, request)
	collections, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs,
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}}, Now: func() time.Time { return newer.WindowEnd },
	})
	if err != nil {
		t.Fatal(err)
	}
	resolverEntered := make(chan struct{})
	resolverRelease := make(chan struct{})
	workerResult := make(chan collectionClaimWorkerResult, 1)
	go func() {
		completed, workerErr := collections.CollectResolvedWithSuccessHook(context.Background(), request.SourceConnectionID, request.QuerySignature, func(context.Context) (domain.CollectionRequest, error) {
			close(resolverEntered)
			<-resolverRelease
			return newer, nil
		}, nil)
		workerResult <- collectionClaimWorkerResult{run: completed, err: workerErr}
	}()
	<-resolverEntered
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: collectionRetryActivatorFake{},
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	retryAttempted := make(chan struct{})
	retryResult := make(chan error, 1)
	go func() {
		close(retryAttempted)
		_, retryErr := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: failed.ID})
		retryResult <- retryErr
	}()
	<-retryAttempted
	waitForAdvisoryWaiter(t, runtime)
	close(resolverRelease)
	worker := <-workerResult
	if worker.err != nil || worker.run.Status != domain.CollectionRunSucceeded || !worker.run.WindowStart.Equal(newer.WindowStart) {
		t.Fatalf("newer worker result = %#v/%v", worker.run, worker.err)
	}
	if retryErr := <-retryResult; retryErr == nil {
		t.Fatal("Retry() unexpectedly succeeded after the newer worker claimed its window")
	}
	var failedStatus string
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, failed.ID).Scan(&failedStatus); err != nil {
		t.Fatal(err)
	}
	var runCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_runs WHERE source_connection_id = $1 AND query_signature = $2`, request.SourceConnectionID, request.QuerySignature).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if failedStatus != "failed" || runCount != 2 {
		t.Fatalf("serialized facts = old run %q count %d, want failed/2", failedStatus, runCount)
	}
}

type collectionClaimWorkerResult struct {
	run domain.CollectionRun
	err error
}

func newerCollectionRequest(t *testing.T, runtime *database.Runtime, request domain.CollectionRequest) domain.CollectionRequest {
	t.Helper()
	newer := request
	newer.Targets = append([]domain.PublishedCollectionTarget(nil), request.Targets...)
	var checkpoint domain.CollectionCheckpoint
	checkpoint.ID = request.Targets[0].Checkpoint.ID
	checkpoint.MonitorSourceID = request.Targets[0].MonitorSourceID
	checkpoint.QueryHash = request.QuerySignature
	if err := runtime.SQL.QueryRow(`
SELECT version, COALESCE(cursor_value, ''), COALESCE(etag, ''), COALESCE(last_modified, ''), next_poll_at
FROM source_checkpoints WHERE id = $1`, checkpoint.ID).Scan(
		&checkpoint.Version, &checkpoint.CursorValue, &checkpoint.ETag, &checkpoint.LastModified, &checkpoint.NextPollAt,
	); err != nil {
		t.Fatal(err)
	}
	newer.WindowStart = checkpoint.NextPollAt
	newer.WindowEnd = checkpoint.NextPollAt.Add(time.Hour)
	newer.Targets[0].Checkpoint = checkpoint
	return newer
}

func waitForAdvisoryWaiter(t *testing.T, runtime *database.Runtime) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err := runtime.SQL.QueryRow(`
SELECT EXISTS (
    SELECT 1 FROM pg_locks
    WHERE locktype = 'advisory'
      AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
      AND NOT granted
)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for a blocked collection advisory lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCollectionControlRetryRejectsNewerQueuedOrRunningRun(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "newer-window", 1)
	runs := sourcepostgres.NewCollectionRepository(runtime)
	failed, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := runs.StartRun(context.Background(), failed.ID, time.Time{}); err != nil || !started {
		t.Fatalf("start failed run: %t/%v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{RunID: failed.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd}); err != nil {
		t.Fatal(err)
	}
	var checkpointVersion int64
	var nextPoll time.Time
	if err := runtime.SQL.QueryRow(`SELECT version, next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&checkpointVersion, &nextPoll); err != nil {
		t.Fatal(err)
	}
	newer := request
	newer.WindowStart = nextPoll
	newer.WindowEnd = nextPoll.Add(time.Hour)
	newer.Targets = append([]domain.PublishedCollectionTarget(nil), request.Targets...)
	newer.Targets[0].Checkpoint.Version = checkpointVersion
	newer.Targets[0].Checkpoint.NextPollAt = nextPoll
	newerRun, created, err := runs.CreateOrReuseRun(context.Background(), newer)
	if err != nil || !created {
		t.Fatalf("create newer run: %#v/%t/%v", newerRun, created, err)
	}
	control, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: runs, Retries: collectionRetryActivatorFake{},
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := identitydomain.Subject{UserID: 1, SessionID: 1, Role: identitydomain.RoleAdmin}
	if _, err := control.Retry(context.Background(), sourceapplication.CollectionRunRetryInput{Subject: admin, ID: failed.ID}); err == nil {
		t.Fatal("Retry(old run) unexpectedly succeeded")
	}
	var failedStatus, newerStatus string
	var persistedNextPoll time.Time
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, failed.ID).Scan(&failedStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, newerRun.ID).Scan(&newerStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT next_poll_at FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&persistedNextPoll); err != nil {
		t.Fatal(err)
	}
	if failedStatus != "failed" || newerStatus != "queued" || !persistedNextPoll.Equal(nextPoll) {
		t.Fatalf("retry rollback = old %q newer %q checkpoint %s", failedStatus, newerStatus, persistedNextPoll)
	}
}

type collectionRetryActivatorFake struct{}

type collectionMonitorAuthorizerFake struct {
	allowedUserID    int64
	allowedMonitorID int64
	calls            int
}

func (fake *collectionMonitorAuthorizerFake) AuthorizeContribution(_ context.Context, subject identitydomain.Subject, monitorID int64) error {
	fake.calls++
	if subject.UserID != fake.allowedUserID || monitorID != fake.allowedMonitorID {
		return errors.New("monitor contribution forbidden")
	}
	return nil
}

func (collectionRetryActivatorFake) Reactivate(context.Context, domain.CollectionRunRetry) error {
	return nil
}

type blockingCollectionRetryActivator struct {
	entered chan struct{}
	release chan struct{}
}

func (activator *blockingCollectionRetryActivator) Reactivate(context.Context, domain.CollectionRunRetry) error {
	close(activator.entered)
	<-activator.release
	return nil
}

func TestCollectionServiceReclaimsQueuedAndStaleRunningRuns(t *testing.T) {
	for _, test := range []struct {
		name       string
		start      bool
		startedAt  time.Time
		wantCursor string
	}{
		{name: "queued", wantCursor: "reclaimed-queued"},
		{name: "stale-running", start: true, startedAt: time.Date(2026, time.July, 16, 11, 0, 0, 0, time.UTC), wantCursor: "reclaimed-running"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := openRuntime(t)
			defer func() { _ = runtime.Close() }()
			request := collectionRequestForService(t, runtime, "reclaim-"+test.name, 1)
			repository := sourcepostgres.NewCollectionRepository(runtime)
			run, created, err := repository.CreateOrReuseRun(context.Background(), request)
			if err != nil || !created {
				t.Fatalf("CreateOrReuseRun() run/created/error = %#v / %t / %v", run, created, err)
			}
			now := time.Date(2026, time.July, 16, 11, 10, 0, 0, time.UTC)
			if test.start {
				if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
					t.Fatalf("StartRun() started/error = %t / %v", started, err)
				}
				if _, err := runtime.SQL.Exec(`UPDATE collection_runs SET started_at = $1 WHERE id = $2`, test.startedAt, run.ID); err != nil {
					t.Fatalf("age running run: %v", err)
				}
			}
			connector := &collectionConnectorFake{result: domain.FetchResult{NextCursor: test.wantCursor}}
			service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
				Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: repository,
				Connectors: collectionConnectorRegistryFake{connector: connector}, Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatalf("NewCollectionService(): %v", err)
			}
			completed, err := service.Collect(context.Background(), request)
			if err != nil || completed.Status != domain.CollectionRunSucceeded || connector.calls.Load() != 1 {
				t.Fatalf("Collect() run/error/fetches = %#v / %v / %d, want reclaimed succeeded run", completed, err, connector.calls.Load())
			}
		})
	}
}

func TestCollectionServiceDoesNotAdvanceTargetWithDifferentCheckpointState(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "checkpoint-state", 2)
	request.Targets[0].Checkpoint.CursorValue = "old-cursor"
	request.Targets[0].Checkpoint.ETag = "old-etag"
	if _, err := runtime.SQL.Exec(`UPDATE source_checkpoints SET cursor_value = $1, etag = $2 WHERE id = $3`, "old-cursor", "old-etag", request.Targets[0].Checkpoint.ID); err != nil {
		t.Fatalf("seed old checkpoint state: %v", err)
	}
	connector := &collectionConnectorFake{result: domain.FetchResult{}}
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: connector}, Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(): %v", err)
	}
	run, err := service.Collect(context.Background(), request)
	if err != nil || run.Status != domain.CollectionRunSucceeded {
		t.Fatalf("Collect() run/error = %#v / %v, want successful run for the fresh checkpoint group", run, err)
	}
	requests := connector.fetchRequests()
	if len(requests) != 1 || requests[0].RequestCursor != "" || requests[0].ETag != "" {
		t.Fatalf("shared request = %#v, want unconditioned fresh-checkpoint fetch", requests)
	}
	var oldStatus, freshStatus, oldCursor, oldETag, freshCursor, freshETag string
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1 AND monitor_source_id = $2`, run.ID, request.Targets[0].MonitorSourceID).Scan(&oldStatus); err != nil {
		t.Fatalf("read old target status: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1 AND monitor_source_id = $2`, run.ID, request.Targets[1].MonitorSourceID).Scan(&freshStatus); err != nil {
		t.Fatalf("read fresh target status: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, ''), COALESCE(etag, '') FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&oldCursor, &oldETag); err != nil {
		t.Fatalf("read old checkpoint: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, ''), COALESCE(etag, '') FROM source_checkpoints WHERE id = $1`, request.Targets[1].Checkpoint.ID).Scan(&freshCursor, &freshETag); err != nil {
		t.Fatalf("read fresh checkpoint: %v", err)
	}
	if oldStatus != "failed" || freshStatus != "succeeded" || oldCursor != "old-cursor" || oldETag != "old-etag" || freshCursor != "" || freshETag != "" {
		t.Fatalf("checkpoint isolation = old=%q/%q/%q fresh=%q/%q/%q", oldStatus, oldCursor, oldETag, freshStatus, freshCursor, freshETag)
	}
}

func collectionRequestForService(t *testing.T, runtime *database.Runtime, name string, targetCount int) domain.CollectionRequest {
	t.Helper()
	connection := sourceConnection("collection-service-" + name)
	connection.HealthStatus = domain.HealthStatusUnknown
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatalf("create collection source: %v", err)
	}
	signature := strings.Repeat("c", 64)
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	targets := make([]domain.PublishedCollectionTarget, 0, targetCount)
	for index := 0; index < targetCount; index++ {
		var monitorID, configID, monitorSourceID, checkpointID, checkpointVersion int64
		suffix := fmt.Sprintf("%s-%d", name, index)
		if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "collection-service-monitor-"+suffix).Scan(&monitorID); err != nil {
			t.Fatalf("create monitor: %v", err)
		}
		if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
			t.Fatalf("create monitor config: %v", err)
		}
		if err := runtime.SQL.QueryRow(`INSERT INTO monitor_sources (config_version_id, source_connection_id, query_signature) VALUES ($1, $2, $3) RETURNING id`, configID, connection.ID, signature).Scan(&monitorSourceID); err != nil {
			t.Fatalf("create monitor source: %v", err)
		}
		if err := runtime.SQL.QueryRow(`INSERT INTO source_checkpoints (monitor_source_id, query_hash, next_poll_at) VALUES ($1, $2, $3) RETURNING id, version`, monitorSourceID, signature, windowStart).Scan(&checkpointID, &checkpointVersion); err != nil {
			t.Fatalf("create source checkpoint: %v", err)
		}
		compiledProfileID := stageCollectionCompiledProfile(t, runtime, monitorID, configID, suffix)
		targets = append(targets, domain.PublishedCollectionTarget{
			MonitorID: monitorID, MonitorSourceID: monitorSourceID, MonitorConfigVersionID: configID,
			CompiledProfileID: compiledProfileID, SourceConnectionID: connection.ID,
			QuerySignature: signature, Terms: []domain.CollectionTerm{{Value: "climate"}}, Languages: []string{"en"},
			CollectionInterval: 5 * time.Minute,
			Checkpoint:         domain.CollectionCheckpoint{ID: checkpointID, Version: checkpointVersion, MonitorSourceID: monitorSourceID, QueryHash: signature, NextPollAt: windowStart},
		})
	}
	return domain.CollectionRequest{SourceConnectionID: connection.ID, QuerySignature: signature, Query: "climate", Languages: []string{"en"}, WindowStart: windowStart, WindowEnd: windowStart.Add(time.Hour), Targets: targets}
}

func collectionRequestsForThreeSourceScan(t *testing.T, runtime *database.Runtime, name string) (int64, []domain.CollectionRequest) {
	t.Helper()
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "collection-service-monitor-"+name).Scan(&monitorID); err != nil {
		t.Fatalf("create multi-source monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create multi-source monitor config: %v", err)
	}
	compiledProfileID := stageCollectionCompiledProfile(t, runtime, monitorID, configID, name)
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	scheduledAt := windowStart.Add(-time.Minute)
	sourceSpecs := []domain.SourceConnection{
		{SourceType: domain.SourceTypeRSS, Name: name + " RSS", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{SourceType: domain.SourceTypeHackerNews, Name: name + " Hacker News", Endpoint: domain.HackerNewsEndpoint, AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
		{SourceType: domain.SourceTypeX, Name: name + " X", Endpoint: domain.XRecentSearchEndpoint, AuthType: domain.AuthTypeBearer, CredentialRef: "env:HOTKEY_TEST_X_TOKEN", Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown},
	}
	requests := make([]domain.CollectionRequest, 0, len(sourceSpecs))
	for index := range sourceSpecs {
		connection := sourceSpecs[index]
		if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
			t.Fatalf("create %s source: %v", connection.SourceType, err)
		}
		signature := strings.Repeat(string(rune('d'+index)), 64)
		var monitorSourceID, checkpointID, checkpointVersion int64
		if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_sources (config_version_id, source_connection_id, query_signature)
VALUES ($1, $2, $3) RETURNING id`, configID, connection.ID, signature).Scan(&monitorSourceID); err != nil {
			t.Fatalf("create %s monitor source: %v", connection.SourceType, err)
		}
		if err := runtime.SQL.QueryRow(`
INSERT INTO source_checkpoints (monitor_source_id, query_hash, next_poll_at)
VALUES ($1, $2, $3) RETURNING id, version`, monitorSourceID, signature, windowStart).Scan(&checkpointID, &checkpointVersion); err != nil {
			t.Fatalf("create %s checkpoint: %v", connection.SourceType, err)
		}
		target := domain.PublishedCollectionTarget{
			MonitorID: monitorID, MonitorSourceID: monitorSourceID, MonitorConfigVersionID: configID,
			CompiledProfileID: compiledProfileID, SourceConnectionID: connection.ID, QuerySignature: signature,
			Terms: []domain.CollectionTerm{{Value: "climate"}}, Languages: []string{"en"}, CollectionInterval: 5 * time.Minute,
			Checkpoint: domain.CollectionCheckpoint{ID: checkpointID, Version: checkpointVersion, MonitorSourceID: monitorSourceID, QueryHash: signature, NextPollAt: windowStart},
		}
		requests = append(requests, domain.CollectionRequest{
			SourceConnectionID: connection.ID, QuerySignature: signature, Query: "climate", Languages: []string{"en"},
			WindowStart: windowStart, WindowEnd: windowStart.Add(time.Hour), ScheduledAt: scheduledAt,
			TriggerType: domain.CollectionTriggerManual, Targets: []domain.PublishedCollectionTarget{target},
		})
	}
	return monitorID, requests
}

func stageCollectionCompiledProfile(t *testing.T, runtime *database.Runtime, monitorID, configID int64, suffix string) int64 {
	t.Helper()
	var draftID, revisionID, riverJobID, previewRunID, previewProfileID, publishedProfileID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`, monitorID, configID).Scan(&draftID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,1,'track climate') RETURNING id`, draftID, monitorID, configID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO river_job (kind,args,state,max_attempts,priority,scheduled_at,unique_key)
VALUES ('analyze_monitor_intent','{}'::jsonb,'completed',3,3,now(),convert_to($1,'UTF8')) RETURNING id`, "collection-profile-"+suffix).Scan(&riverJobID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_analysis_runs (
  monitor_id,draft_id,draft_resource_version,kind,input_hash,profile_version,sample_limit,request_hash,
  idempotency_key,river_job_id,status,queued_at,started_at
) VALUES ($1,$2,1,'preview',repeat('a',64),'preview-v1',25,repeat('b',64),$3,$4,'running',now(),now())
RETURNING id`, monitorID, draftID, "collection-profile-"+suffix, riverJobID).Scan(&previewRunID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,preview_run_id,draft_id,draft_resource_version,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
  structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'preview',$2,$3,$4,1,$5,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
          'halfvec-cosine-v1','entity-hard-rule-v1','canonical-nfc-plaintext-v1','unavailable','semantic_model_unavailable')
RETURNING id`, monitorID, configID, previewRunID, draftID, revisionID).Scan(&previewProfileID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_compiled_clauses (compiled_profile_id,ordinal,operator,field,value,normalized_value,origin)
VALUES ($1,0,'must','term','climate','climate','intent_clause')`, previewProfileID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('9',64),ready_at=now() WHERE id=$1`, previewProfileID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,monitor_version_id,source_preview_compiled_profile_id,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
  structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'published',$2,$2,$3,$4,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
          'halfvec-cosine-v1','entity-hard-rule-v1','canonical-nfc-plaintext-v1','unavailable','semantic_model_unavailable')
RETURNING id`, monitorID, configID, previewProfileID, revisionID).Scan(&publishedProfileID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_compiled_clauses (compiled_profile_id,ordinal,operator,field,value,normalized_value,origin)
VALUES ($1,0,'must','term','climate','climate','intent_clause')`, publishedProfileID); err != nil {
		t.Fatal(err)
	}
	return publishedProfileID
}

func collectionQueueJob(t *testing.T, request domain.CollectionRequest, scheduledAt time.Time, trigger domain.CollectionTriggerType) queue.Job {
	t.Helper()
	representative := request.Targets[0]
	for _, target := range request.Targets[1:] {
		if target.MonitorConfigVersionID < representative.MonitorConfigVersionID {
			representative = target
		}
	}
	args := scheduler.CollectionJobArgs{
		MonitorID: representative.MonitorID, MonitorVersionID: representative.MonitorConfigVersionID,
		CompiledProfileID: representative.CompiledProfileID, SourceConnectionID: request.SourceConnectionID,
		WindowStart: request.WindowStart, WindowEnd: request.WindowEnd, InputHash: request.QuerySignature,
		TriggerType: string(trigger),
	}
	encoded, err := scheduler.EncodeCollectionJobArgs(args)
	if err != nil {
		t.Fatalf("EncodeCollectionJobArgs(): %v", err)
	}
	uniqueKey := scheduler.CollectionUniqueKey(args.MonitorID, args.MonitorVersionID, args.CompiledProfileID,
		args.SourceConnectionID, args.WindowStart, args.WindowEnd)
	priority := 1
	if trigger == domain.CollectionTriggerManual {
		uniqueKey = scheduler.ManualCollectionUniqueKey(args.MonitorID, args.MonitorVersionID, args.CompiledProfileID,
			args.SourceConnectionID, scheduledAt)
		priority = 2
	}
	return queue.Job{
		Kind: queue.KindCollectSource, UniqueKey: uniqueKey, DurableArgs: encoded,
		ScheduledAt: scheduledAt, MaxAttempts: 3, Priority: priority,
	}
}

type collectionConnectorRegistryFake struct{ connector domain.Connector }

func (registry collectionConnectorRegistryFake) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	return registry.connector, nil
}

type collectionConnectorRegistryByTypeFake struct {
	connectors map[domain.SourceType]domain.Connector
}

func (registry collectionConnectorRegistryByTypeFake) Resolve(_ context.Context, connection domain.SourceConnection) (domain.Connector, error) {
	connector, found := registry.connectors[connection.SourceType]
	if !found {
		return nil, fmt.Errorf("connector %q is not configured", connection.SourceType)
	}
	return connector, nil
}

type collectionConnectorFake struct {
	calls    atomic.Int32
	requests []domain.FetchRequest
	mu       sync.Mutex
	result   domain.FetchResult
	err      error
	results  []domain.FetchResult
	errors   []error
	health   domain.HealthResult
}

func (connector *collectionConnectorFake) Validate(context.Context, domain.SourceConnection) error {
	return nil
}

func (connector *collectionConnectorFake) Fetch(_ context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	call := int(connector.calls.Add(1)) - 1
	connector.mu.Lock()
	defer connector.mu.Unlock()
	connector.requests = append(connector.requests, request)
	if call < len(connector.results) || call < len(connector.errors) {
		var result domain.FetchResult
		var err error
		if call < len(connector.results) {
			result = connector.results[call]
		}
		if call < len(connector.errors) {
			err = connector.errors[call]
		}
		return result, err
	}
	return connector.result, connector.err
}

func (connector *collectionConnectorFake) Health(context.Context, domain.SourceConnection) domain.HealthResult {
	return connector.health
}

type collectionMetricsFake struct{ values [][2]string }

func (metrics *collectionMetricsFake) RecordCollectionOperation(operation, outcome string) {
	metrics.values = append(metrics.values, [2]string{operation, outcome})
}

func (metrics *collectionMetricsFake) recorded(operation, outcome string) bool {
	for _, value := range metrics.values {
		if value == [2]string{operation, outcome} {
			return true
		}
	}
	return false
}

func (connector *collectionConnectorFake) fetchRequests() []domain.FetchRequest {
	connector.mu.Lock()
	defer connector.mu.Unlock()
	return append([]domain.FetchRequest(nil), connector.requests...)
}
