package application_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func TestCollectionServiceFetchesOnceAndDurablyReconcilesEveryTarget(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "shared-capture", 2)
	connector := &collectionConnectorFake{result: domain.FetchResult{
		Items: []domain.SourceItem{{
			SourceCode: "rss", ExternalID: "post-42", ContentType: "article", Title: "Safe title",
			Body: "body retained from the source Feed", ObservedAt: time.Date(2026, time.July, 16, 8, 5, 0, 0, time.UTC),
			Metrics: domain.SourceMetrics{ViewCount: domain.KnownMetric(12), CommentCount: domain.KnownMetric(3)}, RawPayload: []byte(`{"authorization":"never-persist"}`),
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
	if strings.Contains(payload, "authorization") || strings.Contains(payload, "never-persist") || !strings.Contains(payload, "body retained from the source Feed") {
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

func TestCollectionServicePersistsAuthenticationAndPermanentFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		kind domain.CollectionErrorKind
	}{
		{name: "authentication", kind: domain.CollectionErrorAuthentication},
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
		Metrics:    metrics, Now: func() time.Time { return checkedAt },
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
		targets = append(targets, domain.PublishedCollectionTarget{
			MonitorSourceID: monitorSourceID, MonitorConfigVersionID: configID, SourceConnectionID: connection.ID,
			QuerySignature: signature, Terms: []domain.CollectionTerm{{Value: "climate"}}, Languages: []string{"en"},
			CollectionInterval: 5 * time.Minute,
			Checkpoint:         domain.CollectionCheckpoint{ID: checkpointID, Version: checkpointVersion, MonitorSourceID: monitorSourceID, QueryHash: signature, NextPollAt: windowStart},
		})
	}
	return domain.CollectionRequest{SourceConnectionID: connection.ID, QuerySignature: signature, Query: "climate", Languages: []string{"en"}, WindowStart: windowStart, WindowEnd: windowStart.Add(time.Hour), Targets: targets}
}

type collectionConnectorRegistryFake struct{ connector domain.Connector }

func (registry collectionConnectorRegistryFake) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	return registry.connector, nil
}

type collectionConnectorFake struct {
	calls    atomic.Int32
	requests []domain.FetchRequest
	mu       sync.Mutex
	result   domain.FetchResult
	err      error
	health   domain.HealthResult
}

func (connector *collectionConnectorFake) Validate(context.Context, domain.SourceConnection) error {
	return nil
}

func (connector *collectionConnectorFake) Fetch(_ context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	connector.calls.Add(1)
	connector.mu.Lock()
	connector.requests = append(connector.requests, request)
	connector.mu.Unlock()
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
