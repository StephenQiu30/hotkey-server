package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func TestCollectionRepositoryCreateOrReuseRunIsRaceSafeAndCreatesAllTargets(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "create-or-reuse", 2)

	const callers = 8
	results := make(chan struct {
		run     domain.CollectionRun
		created bool
		err     error
	}, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			run, created, err := repository.CreateOrReuseRun(context.Background(), request)
			results <- struct {
				run     domain.CollectionRun
				created bool
				err     error
			}{run: run, created: created, err: err}
		}()
	}
	group.Wait()
	close(results)

	var runID int64
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("CreateOrReuseRun() error = %v", result.err)
		}
		if runID == 0 {
			runID = result.run.ID
		}
		if result.run.ID != runID || result.run.Status != domain.CollectionRunQueued {
			t.Fatalf("CreateOrReuseRun() result = %#v, want same queued run", result.run)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
	var runs, targets int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_runs`).Scan(&runs); err != nil {
		t.Fatalf("count collection runs: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_targets WHERE collection_run_id = $1`, runID).Scan(&targets); err != nil {
		t.Fatalf("count collection targets: %v", err)
	}
	if runs != 1 || targets != len(request.Targets) {
		t.Fatalf("persisted runs/targets = %d/%d, want 1/%d", runs, targets, len(request.Targets))
	}
}

func TestCollectionRepositoryRollsBackCaptureBeforeCheckpointAdvance(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "capture-rollback", 1)
	run, created, err := repository.CreateOrReuseRun(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("CreateOrReuseRun() run/created/error = %#v / %t / %v", run, created, err)
	}
	if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun() started/error = %t / %v", started, err)
	}
	if _, err := runtime.SQL.Exec(`
CREATE OR REPLACE FUNCTION reject_collection_rollback_item()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.external_id = 'rollback-item' THEN
        RAISE EXCEPTION 'forced capture write failure';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER collection_run_items_rollback_test
BEFORE INSERT ON collection_run_items
FOR EACH ROW EXECUTE FUNCTION reject_collection_rollback_item();`); err != nil {
		t.Fatalf("install rollback trigger: %v", err)
	}
	defer func() {
		_, _ = runtime.SQL.Exec(`DROP TRIGGER IF EXISTS collection_run_items_rollback_test ON collection_run_items; DROP FUNCTION IF EXISTS reject_collection_rollback_item();`)
	}()

	policy := domain.CapturePolicy{Version: domain.CapturedItemVersionV2, RawPayloadDisposition: domain.RawPayloadDiscarded}
	items := make([]domain.CapturedItem, 0, 2)
	for _, externalID := range []string{"first-item", "rollback-item"} {
		item, err := policy.Capture(domain.SourceItem{SourceCode: "rss", ExternalID: externalID, ContentType: "article", ObservedAt: request.WindowStart})
		if err != nil {
			t.Fatalf("Capture(%q): %v", externalID, err)
		}
		items = append(items, item)
	}
	if _, err := repository.PersistSuccess(context.Background(), domain.CollectionRunSuccess{
		RunID: run.ID, Targets: request.Targets, Items: items, Result: domain.FetchResult{NextCursor: "must-not-advance"}, CompletedAt: request.WindowEnd,
	}); err == nil {
		t.Fatal("PersistSuccess() error = nil, want capture transaction failure")
	}
	var itemCount, reconciliationCount int
	var cursor, runStatus, targetStatus string
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id = $1`, run.ID).Scan(&itemCount); err != nil {
		t.Fatalf("count rolled back items: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_target_items WHERE collection_run_id = $1`, run.ID).Scan(&reconciliationCount); err != nil {
		t.Fatalf("count rolled back reconciliation: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(cursor_value, '') FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&cursor); err != nil {
		t.Fatalf("read rolled back checkpoint: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT status FROM collection_runs WHERE id = $1`, run.ID).Scan(&runStatus); err != nil {
		t.Fatalf("read rolled back run: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT target_status FROM collection_run_targets WHERE collection_run_id = $1`, run.ID).Scan(&targetStatus); err != nil {
		t.Fatalf("read rolled back target: %v", err)
	}
	if itemCount != 0 || reconciliationCount != 0 || cursor != "" || runStatus != "running" || targetStatus != "queued" {
		t.Fatalf("rollback state = items=%d reconcile=%d cursor=%q run=%q target=%q", itemCount, reconciliationCount, cursor, runStatus, targetStatus)
	}
}

func TestCollectionRepositoryMakesCapturedItemPersistenceIdempotent(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "item-idempotency", 1)
	run, _, err := repository.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateOrReuseRun(): %v", err)
	}
	if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun() started/error = %t / %v", started, err)
	}
	captured, err := (domain.CapturePolicy{Version: domain.CapturedItemVersionV2, RawPayloadDisposition: domain.RawPayloadDiscarded}).Capture(domain.SourceItem{
		SourceCode: "rss", ExternalID: "retry-safe-item", ContentType: "article", ObservedAt: request.WindowStart,
	})
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	success := domain.CollectionRunSuccess{RunID: run.ID, Targets: request.Targets, Items: []domain.CapturedItem{captured}, Result: domain.FetchResult{NextCursor: "retry-safe-cursor"}, CompletedAt: request.WindowEnd}
	first, err := repository.PersistSuccess(context.Background(), success)
	if err != nil {
		t.Fatalf("PersistSuccess(first): %v", err)
	}
	second, err := repository.PersistSuccess(context.Background(), success)
	if err != nil || first.ID != second.ID || second.Status != domain.CollectionRunSucceeded {
		t.Fatalf("PersistSuccess(replay) run/error = %#v / %v, want same succeeded run", second, err)
	}
	var items, reconciled int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id = $1`, run.ID).Scan(&items); err != nil {
		t.Fatalf("count replayed items: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_target_items WHERE collection_run_id = $1`, run.ID).Scan(&reconciled); err != nil {
		t.Fatalf("count replayed target items: %v", err)
	}
	if items != 1 || reconciled != 1 {
		t.Fatalf("replayed items/reconciliation = %d/%d, want 1/1", items, reconciled)
	}
}

func TestCollectionRepositoryReadsAndBindsCapturedItemsWithSourceOwnership(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "ingestion-boundary", 1)
	run, created, err := repository.CreateOrReuseRun(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("CreateOrReuseRun() run/created/error = %#v / %t / %v", run, created, err)
	}
	if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun() started/error = %t / %v", started, err)
	}
	captured, err := (domain.CapturePolicy{Version: domain.CapturedItemVersionV2, RawPayloadDisposition: domain.RawPayloadDiscarded}).Capture(domain.SourceItem{
		SourceCode: "rss", ExternalID: "ingestion-boundary-item", ContentType: "article", Title: "Capture boundary", URL: "https://example.test/capture-boundary", ObservedAt: request.WindowStart,
	})
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	if _, err := repository.PersistSuccess(context.Background(), domain.CollectionRunSuccess{
		RunID: run.ID, Targets: request.Targets, Items: []domain.CapturedItem{captured}, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatalf("PersistSuccess(): %v", err)
	}

	page, err := repository.ListUnboundCaptured(context.Background(), domain.CapturedItemQuery{RunID: run.ID, Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListUnboundCaptured() page/error = %#v / %v, want one pending capture", page, err)
	}
	item := page.Items[0]
	if item.RunID != run.ID || item.SourceConnectionID != request.SourceConnectionID || item.Item.Version != domain.CapturedItemVersionV2 {
		t.Fatalf("captured item = %#v, want source-owned v2 run item", item)
	}

	var wrongSourceID, wrongContentID, contentID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'ingestion-wrong-source', 'https://example.test/wrong') RETURNING id`).Scan(&wrongSourceID); err != nil {
		t.Fatalf("create mismatched source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (source_connection_id, external_id, content_type, canonical_url, published_at, fetched_at, dedupe_key)
VALUES ($1, 'wrong-content', 'article', 'https://example.test/wrong-content', $2, $2, $3)
RETURNING id`, wrongSourceID, request.WindowStart, strings.Repeat("b", 64)).Scan(&wrongContentID); err != nil {
		t.Fatalf("create mismatched content: %v", err)
	}
	if err := repository.BindContent(context.Background(), domain.CapturedContentBinding{
		CollectionItemID: item.ID, RunID: run.ID, SourceConnectionID: request.SourceConnectionID, ContentID: wrongContentID,
	}); err == nil {
		t.Fatal("BindContent(mismatched source) error = nil, want foreign-key rejection")
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (source_connection_id, external_id, content_type, canonical_url, published_at, fetched_at, dedupe_key)
VALUES ($1, 'bound-content', 'article', 'https://example.test/bound-content', $2, $2, $3)
RETURNING id`, request.SourceConnectionID, request.WindowStart, strings.Repeat("c", 64)).Scan(&contentID); err != nil {
		t.Fatalf("create matching content: %v", err)
	}
	for _, outcome := range []string{"skipped", "failed"} {
		var nonCapturedItemID int64
		if err := runtime.SQL.QueryRow(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, 'rss', $3, 'article', 'v1', '{"title":"not-ingestable"}'::jsonb, $4, 'discarded', $5, $6)
RETURNING id`, run.ID, request.SourceConnectionID, "non-captured-"+outcome, strings.Repeat(outcome[:1], 64), outcome, request.WindowStart).Scan(&nonCapturedItemID); err != nil {
			t.Fatalf("insert %s collection item: %v", outcome, err)
		}
		if err := repository.BindContent(context.Background(), domain.CapturedContentBinding{
			CollectionItemID: nonCapturedItemID, RunID: run.ID, SourceConnectionID: request.SourceConnectionID, ContentID: contentID,
		}); !errors.Is(err, sharedrepository.ErrConflict) {
			t.Fatalf("BindContent(%s item) error = %v, want conflict", outcome, err)
		}
		if err := repository.MarkIngestionFailure(context.Background(), domain.CapturedIngestionFailure{
			CollectionItemID: nonCapturedItemID, RunID: run.ID, SourceConnectionID: request.SourceConnectionID, Code: "not_ingestable",
		}); !errors.Is(err, sharedrepository.ErrConflict) {
			t.Fatalf("MarkIngestionFailure(%s item) error = %v, want conflict", outcome, err)
		}
		var nonCapturedContentID any
		var nonCapturedStatus, nonCapturedError string
		if err := runtime.SQL.QueryRow(`
SELECT content_id, ingestion_status, COALESCE(ingestion_error_code, '')
FROM collection_run_items
WHERE id = $1`, nonCapturedItemID).Scan(&nonCapturedContentID, &nonCapturedStatus, &nonCapturedError); err != nil {
			t.Fatalf("read %s collection item after ingestion calls: %v", outcome, err)
		}
		if nonCapturedContentID != nil || nonCapturedStatus != "pending" || nonCapturedError != "" {
			t.Fatalf("%s collection item state = %#v/%q/%q, want nil/pending/empty", outcome, nonCapturedContentID, nonCapturedStatus, nonCapturedError)
		}
	}
	if err := repository.MarkIngestionFailure(context.Background(), domain.CapturedIngestionFailure{
		CollectionItemID: item.ID, RunID: run.ID, SourceConnectionID: request.SourceConnectionID, Code: "normalize_invalid",
	}); err != nil {
		t.Fatalf("MarkIngestionFailure(): %v", err)
	}
	if page, err := repository.ListUnboundCaptured(context.Background(), domain.CapturedItemQuery{RunID: run.ID, Limit: 1}); err != nil || len(page.Items) != 0 {
		t.Fatalf("ListUnboundCaptured(default) page/error = %#v / %v, want failed item excluded", page, err)
	}
	if page, err := repository.ListUnboundCaptured(context.Background(), domain.CapturedItemQuery{RunID: run.ID, Limit: 1, IncludeFailed: true}); err != nil || len(page.Items) != 1 {
		t.Fatalf("ListUnboundCaptured(retry) page/error = %#v / %v, want failed item", page, err)
	}
	if err := repository.BindContent(context.Background(), domain.CapturedContentBinding{
		CollectionItemID: item.ID, RunID: run.ID, SourceConnectionID: request.SourceConnectionID, ContentID: contentID,
	}); err != nil {
		t.Fatalf("BindContent(): %v", err)
	}
	var status, errorCode, outcome string
	if err := runtime.SQL.QueryRow(`SELECT ingestion_status, COALESCE(ingestion_error_code, ''), outcome FROM collection_run_items WHERE id = $1`, item.ID).Scan(&status, &errorCode, &outcome); err != nil {
		t.Fatalf("read bound capture: %v", err)
	}
	if status != "succeeded" || errorCode != "" || outcome != "captured" {
		t.Fatalf("bound capture status/error/outcome = %q/%q/%q, want succeeded/empty/captured", status, errorCode, outcome)
	}
	if err := repository.BindContent(context.Background(), domain.CapturedContentBinding{
		CollectionItemID: item.ID, RunID: run.ID, SourceConnectionID: request.SourceConnectionID, ContentID: contentID,
	}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("BindContent(replay) error = %v, want conflict", err)
	}
}

func TestCollectionRepositoryMapsLegacyCaptureZeroMetricsToUnknown(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "legacy-metrics", 1)
	run, created, err := repository.CreateOrReuseRun(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("CreateOrReuseRun() run/created/error = %#v / %t / %v", run, created, err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_items
    (run_id, source_connection_id, source_code, external_id, content_type, captured_item_version,
     captured_item, payload_hash, raw_payload_disposition, outcome, observed_at)
VALUES ($1, $2, 'rss', 'legacy-metrics', 'article', 'v1',
        '{"version":"v1","source_code":"rss","external_id":"legacy-metrics","content_type":"article","title":"legacy","url":"https://example.test/legacy","observed_at":"2026-07-16T08:00:00Z","metrics":{"ViewCount":0,"LikeCount":9,"CommentCount":0,"ShareCount":4},"raw_payload_disposition":"discarded"}'::jsonb,
        $3, 'discarded', 'captured', $4)`, run.ID, request.SourceConnectionID, strings.Repeat("d", 64), request.WindowStart); err != nil {
		t.Fatalf("insert legacy captured item: %v", err)
	}
	page, err := repository.ListUnboundCaptured(context.Background(), domain.CapturedItemQuery{RunID: run.ID, Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListUnboundCaptured() page/error = %#v / %v, want one legacy item", page, err)
	}
	metrics := page.Items[0].Item.Metrics
	if metrics.ViewCount != nil || metrics.CommentCount != nil {
		t.Fatalf("legacy zero metrics = %#v, want unknown nil values", metrics)
	}
	if metrics.LikeCount == nil || *metrics.LikeCount != 9 || metrics.ShareCount == nil || *metrics.ShareCount != 4 {
		t.Fatalf("legacy positive metrics = %#v, want 9/4", metrics)
	}
}

func TestCollectionRepositoryListsSafeSummariesAndRequeuesOnlyTerminalFailures(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "admin-retry", 2)
	run, created, err := repository.CreateOrReuseRun(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("CreateOrReuseRun() run/created/error = %#v / %t / %v", run, created, err)
	}
	if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun() started/error = %t / %v", started, err)
	}
	if _, err := repository.PersistFailure(context.Background(), domain.CollectionRunFailure{
		RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd,
	}); err != nil {
		t.Fatalf("PersistFailure(): %v", err)
	}

	page, err := repository.ListRuns(context.Background(), domain.CollectionRunListQuery{Limit: 1})
	if err != nil {
		t.Fatalf("ListRuns(): %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != run.ID || page.Items[0].Status != domain.CollectionRunFailed || page.Items[0].ErrorCode != string(domain.CollectionErrorTemporary) || len(page.Items[0].Targets) != 2 {
		t.Fatalf("safe run page = %#v, want one failed run with two target summaries", page)
	}
	for _, target := range page.Items[0].Targets {
		if target.Status != domain.CollectionRunFailed || target.ErrorCode != string(domain.CollectionErrorTemporary) {
			t.Fatalf("failed target summary = %#v", target)
		}
	}
	if _, err := repository.ListRuns(context.Background(), domain.CollectionRunListQuery{Cursor: "not-a-cursor"}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ListRuns(invalid cursor) error = %v, want invalid input", err)
	}

	retried, err := repository.RetryRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("RetryRun(): %v", err)
	}
	if retried.Status != domain.CollectionRunQueued || retried.ErrorCode != "" || retried.StartedAt != nil || retried.FinishedAt != nil || len(retried.Targets) != 2 {
		t.Fatalf("retried summary = %#v, want reset queued run", retried)
	}
	for _, target := range retried.Targets {
		if target.Status != domain.CollectionRunQueued || target.ErrorCode != "" || target.CandidateCount != 0 || target.AcceptedCount != 0 || target.RejectedCount != 0 {
			t.Fatalf("retried target = %#v, want reset queued target", target)
		}
	}
	var triggerType, status string
	var retryAfter, startedAt, finishedAt any
	if err := runtime.SQL.QueryRow(`SELECT trigger_type, status, retry_after, started_at, finished_at FROM collection_runs WHERE id = $1`, run.ID).Scan(&triggerType, &status, &retryAfter, &startedAt, &finishedAt); err != nil {
		t.Fatalf("read requeued run: %v", err)
	}
	if triggerType != "retry" || status != "queued" || retryAfter != nil || startedAt != nil || finishedAt != nil {
		t.Fatalf("requeued database state = trigger=%q status=%q retry=%v started=%v finished=%v", triggerType, status, retryAfter, startedAt, finishedAt)
	}
	if _, err := repository.RetryRun(context.Background(), run.ID); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("RetryRun(queued) error = %v, want conflict", err)
	}
}

func TestCollectionRepositoryRetryRepairsCheckpointConflictTargetReconciliation(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewCollectionRepository(runtime)
	request := collectionRequestForRepository(t, runtime, "retry-reconciliation", 1)
	run, _, err := repository.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateOrReuseRun(): %v", err)
	}
	if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun(first) started/error = %t / %v", started, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE source_checkpoints SET version = version + 1 WHERE id = $1`, request.Targets[0].Checkpoint.ID); err != nil {
		t.Fatalf("make checkpoint stale: %v", err)
	}
	item, err := (domain.CapturePolicy{Version: domain.CapturedItemVersionV2, RawPayloadDisposition: domain.RawPayloadDiscarded}).Capture(domain.SourceItem{
		SourceCode: "rss", ExternalID: "retry-reconciliation-item", ContentType: "article", ObservedAt: request.WindowStart,
	})
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	failed, err := repository.PersistSuccess(context.Background(), domain.CollectionRunSuccess{
		RunID: run.ID, Targets: request.Targets, Items: []domain.CapturedItem{item}, CompletedAt: request.WindowEnd,
	})
	if err != nil || failed.Status != domain.CollectionRunFailed {
		t.Fatalf("PersistSuccess(stale checkpoint) run/error = %#v / %v, want target capture failed run", failed, err)
	}
	var outcome, reason string
	if err := runtime.SQL.QueryRow(`SELECT outcome, COALESCE(reason_code, '') FROM collection_run_target_items WHERE collection_run_id = $1`, run.ID).Scan(&outcome, &reason); err != nil {
		t.Fatalf("read failed reconciliation: %v", err)
	}
	if outcome != "failed" || reason != "checkpoint_conflict" {
		t.Fatalf("failed reconciliation = outcome=%q reason=%q", outcome, reason)
	}

	if _, err := repository.RetryRun(context.Background(), run.ID); err != nil {
		t.Fatalf("RetryRun(): %v", err)
	}
	if _, started, err := repository.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun(retry) started/error = %t / %v", started, err)
	}
	var checkpointVersion int64
	if err := runtime.SQL.QueryRow(`SELECT version FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&checkpointVersion); err != nil {
		t.Fatalf("read refreshed checkpoint version: %v", err)
	}
	retryTargets := append([]domain.PublishedCollectionTarget(nil), request.Targets...)
	retryTargets[0].Checkpoint.Version = checkpointVersion
	succeeded, err := repository.PersistSuccess(context.Background(), domain.CollectionRunSuccess{
		RunID: run.ID, Targets: retryTargets, Items: []domain.CapturedItem{item}, CompletedAt: request.WindowEnd.Add(time.Minute),
	})
	if err != nil || succeeded.Status != domain.CollectionRunSucceeded {
		t.Fatalf("PersistSuccess(retry) run/error = %#v / %v, want succeeded retry", succeeded, err)
	}
	if err := runtime.SQL.QueryRow(`SELECT outcome, COALESCE(reason_code, '') FROM collection_run_target_items WHERE collection_run_id = $1`, run.ID).Scan(&outcome, &reason); err != nil {
		t.Fatalf("read repaired reconciliation: %v", err)
	}
	if outcome != "captured" || reason != "" {
		t.Fatalf("repaired reconciliation = outcome=%q reason=%q, want captured with no reason", outcome, reason)
	}
}

func collectionRequestForRepository(t *testing.T, runtime *database.Runtime, name string, targetCount int) domain.CollectionRequest {
	t.Helper()
	connection := sourceConnection("collection-" + name)
	sources := sourcepostgres.NewRepository(runtime)
	if err := sources.Create(context.Background(), &connection); err != nil {
		t.Fatalf("create collection source: %v", err)
	}
	signature := strings.Repeat("a", 64)
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	targets := make([]domain.PublishedCollectionTarget, 0, targetCount)
	for index := 0; index < targetCount; index++ {
		suffix := fmt.Sprintf("%s-%d", name, index)
		var monitorID, configID, monitorSourceID, checkpointID, checkpointVersion int64
		if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "collection-monitor-"+suffix).Scan(&monitorID); err != nil {
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
