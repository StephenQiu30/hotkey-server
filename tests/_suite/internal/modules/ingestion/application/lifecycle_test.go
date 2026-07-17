package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func TestDeleteBySourceItemMarksContentDeletedBeforeRetryingEvidenceDeletion(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	runID, sourceID := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("delete-me", "article", "Delete evidence", "delete lifecycle evidence"),
	})
	store := newEvidenceStoreFake()
	ingest := newLifecycleService(t, runtime, store)
	if result, err := ingest.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 1}); err != nil || result.Bound != 1 {
		t.Fatalf("IngestRun() result/error = %#v / %v, want one bound item", result, err)
	}

	failingStore := &failFirstLifecycleDeleteStore{EvidenceStore: store}
	service := newLifecycleService(t, runtime, failingStore)
	first, err := service.DeleteBySourceItem(context.Background(), sourceID, "delete-me")
	if err == nil {
		t.Fatal("DeleteBySourceItem(first) error = nil, want surfaced evidence deletion failure")
	}
	if !first.ContentChanged || first.AssetsDeletePending != 1 || first.AssetsDeleted != 0 {
		t.Fatalf("DeleteBySourceItem(first) result = %#v, want deleted content and one pending asset", first)
	}
	assertContentAndAssetLifecycle(t, runtime, sourceID, "delete-me", ingestiondomain.ContentStatusDeleted, ingestiondomain.AssetStatusDeletePending)
	assertNoActiveContent(t, runtime)

	second, err := service.DeleteBySourceItem(context.Background(), sourceID, "delete-me")
	if err != nil {
		t.Fatalf("DeleteBySourceItem(retry) error = %v", err)
	}
	if second.ContentChanged || second.AssetsDeleted != 1 || second.AssetsDeletePending != 0 {
		t.Fatalf("DeleteBySourceItem(retry) result = %#v, want idempotent content and one deleted asset", second)
	}
	assertContentAndAssetLifecycle(t, runtime, sourceID, "delete-me", ingestiondomain.ContentStatusDeleted, ingestiondomain.AssetStatusDeleted)
	if len(store.objects) != 0 {
		t.Fatalf("evidence objects after successful retry = %#v, want none", store.objects)
	}

	third, err := service.DeleteBySourceItem(context.Background(), sourceID, "delete-me")
	if err != nil {
		t.Fatalf("DeleteBySourceItem(repeated) error = %v", err)
	}
	if third.ContentChanged || third.AssetsDeleted != 0 || third.AssetsDeletePending != 0 {
		t.Fatalf("DeleteBySourceItem(repeated) result = %#v, want no-op", third)
	}
}

func TestDeleteBySourceItemMissingSourceItemIsIdempotentNoOp(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	store := newEvidenceStoreFake()
	service := newLifecycleService(t, runtime, store)

	result, err := service.DeleteBySourceItem(context.Background(), 987654, "does-not-exist")
	if err != nil {
		t.Fatalf("DeleteBySourceItem(missing) error = %v", err)
	}
	if result.ContentChanged || result.AssetsDeleted != 0 || result.AssetsDeletePending != 0 || len(store.objects) != 0 {
		t.Fatalf("DeleteBySourceItem(missing) result/store = %#v / %#v, want no-op", result, store.objects)
	}
}

func TestDeleteBySourceItemSerializesTombstoneBeforePausedCaptureBinds(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	firstRunID, sourceID := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("delete-race", "article", "Existing content", ""),
	})
	store := newEvidenceStoreFake()
	initial := newLifecycleService(t, runtime, store)
	if result, err := initial.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: firstRunID, Limit: 1}); err != nil || result.Bound != 1 {
		t.Fatalf("IngestRun(initial) result/error = %#v / %v, want title-only Content", result, err)
	}

	secondRunID := seedLifecycleReplayRun(t, runtime, sourceID, capturedItem("delete-race", "article", "Paused capture", "must be compensated after tombstone"))
	blockingStore := newLifecyclePutBlockingStore(store)
	paused := newLifecycleService(t, runtime, blockingStore)
	resultCh := make(chan struct {
		result ingestionapplication.IngestRunResult
		err    error
	}, 1)
	go func() {
		result, err := paused.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: secondRunID, Limit: 1})
		resultCh <- struct {
			result ingestionapplication.IngestRunResult
			err    error
		}{result, err}
	}()
	blockingStore.waitForPut(t)

	deleteService := newLifecycleService(t, runtime, store)
	deleted, err := deleteService.DeleteBySourceItem(context.Background(), sourceID, "delete-race")
	if err != nil || !deleted.ContentChanged {
		t.Fatalf("DeleteBySourceItem() result/error = %#v / %v, want tombstone committed before capture transaction", deleted, err)
	}
	blockingStore.releasePut()
	select {
	case result := <-resultCh:
		if result.err != nil || result.result.Processed != 1 || result.result.Bound != 0 || result.result.Failed != 1 || result.result.Uploaded != 0 {
			t.Fatalf("IngestRun(paused) result/error = %#v / %v, want classified tombstone failure", result.result, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("paused capture did not complete after release")
	}

	assertContentAndAssetLifecycleWithoutAsset(t, runtime, sourceID, "delete-race", ingestiondomain.ContentStatusDeleted)
	receipts, err := store.ListPrefix(context.Background(), fmt.Sprintf("evidence/v1/%d/", sourceID))
	if err != nil {
		t.Fatalf("ListPrefix() after compensation: %v", err)
	}
	if len(receipts) != 0 || blockingStore.putCount() != 1 {
		t.Fatalf("evidence after tombstone interleaving = receipts=%#v puts=%d, want absent compensated object after one put", receipts, blockingStore.putCount())
	}
	var status, failureCode string
	var contentID *int64
	if err := runtime.SQL.QueryRow(`
SELECT ingestion_status, ingestion_error_code, content_id
FROM collection_run_items
WHERE run_id = $1`, secondRunID).Scan(&status, &failureCode, &contentID); err != nil {
		t.Fatalf("read paused capture state: %v", err)
	}
	if status != "failed" || failureCode != "content_deleted" || contentID != nil {
		t.Fatalf("paused capture state = status=%q code=%q content=%v, want failed/content_deleted/unbound", status, failureCode, contentID)
	}
}

type failFirstLifecycleDeleteStore struct {
	ingestiondomain.EvidenceStore
	mu       sync.Mutex
	deleteAt int
}

func (store *failFirstLifecycleDeleteStore) Delete(ctx context.Context, objectKey string) error {
	store.mu.Lock()
	store.deleteAt++
	fail := store.deleteAt == 1
	store.mu.Unlock()
	if fail {
		return errors.New("injected lifecycle delete failure")
	}
	return store.EvidenceStore.Delete(ctx, objectKey)
}

func newLifecycleService(t *testing.T, runtime *database.Runtime, evidence ingestiondomain.EvidenceStore) *ingestionapplication.Service {
	t.Helper()
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: newCapturedItemReader(t, runtime), Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: evidence,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func assertContentAndAssetLifecycle(t *testing.T, runtime *database.Runtime, sourceID int64, externalID string, wantContent ingestiondomain.ContentStatus, wantAsset ingestiondomain.AssetStatus) {
	t.Helper()
	var contentStatus, assetStatus string
	err := runtime.SQL.QueryRow(`
SELECT content.content_status, asset.object_status
FROM contents AS content
JOIN content_assets AS asset ON asset.content_id = content.id
WHERE content.source_connection_id = $1 AND content.external_id = $2`, sourceID, externalID).Scan(&contentStatus, &assetStatus)
	if err != nil {
		t.Fatalf("read content/asset lifecycle: %v", err)
	}
	if contentStatus != string(wantContent) || assetStatus != string(wantAsset) {
		t.Fatalf("content/asset lifecycle = %q/%q, want %q/%q", contentStatus, assetStatus, wantContent, wantAsset)
	}
}

func assertContentAndAssetLifecycleWithoutAsset(t *testing.T, runtime *database.Runtime, sourceID int64, externalID string, wantContent ingestiondomain.ContentStatus) {
	t.Helper()
	var contentStatus string
	var assets int
	if err := runtime.SQL.QueryRow(`SELECT content_status FROM contents WHERE source_connection_id = $1 AND external_id = $2`, sourceID, externalID).Scan(&contentStatus); err != nil {
		t.Fatalf("read tombstoned content: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT count(*)
FROM content_assets AS asset
JOIN contents AS content ON content.id = asset.content_id
WHERE content.source_connection_id = $1 AND content.external_id = $2`, sourceID, externalID).Scan(&assets); err != nil {
		t.Fatalf("count tombstoned content assets: %v", err)
	}
	if contentStatus != string(wantContent) || assets != 0 {
		t.Fatalf("tombstoned content/assets = %q/%d, want %q/no assets", contentStatus, assets, wantContent)
	}
}

func assertNoActiveContent(t *testing.T, runtime *database.Runtime) {
	t.Helper()
	repository := ingestionpostgres.NewContentRepository(runtime)
	page, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("ListActive() items = %#v, want tombstoned content excluded", page.Items)
	}
}

func deterministicEvidence(t *testing.T, store ingestiondomain.EvidenceStore, sourceID int64, text string) ingestiondomain.EvidenceReceipt {
	t.Helper()
	digest := sha256.Sum256([]byte(text))
	receipt, err := store.PutText(context.Background(), ingestiondomain.EvidenceObject{
		SourceConnectionID: sourceID,
		ObjectKey:          fmt.Sprintf("evidence/v1/%d/%x/%x.txt", sourceID, digest[:1], digest),
		Text:               text,
		SHA256:             fmt.Sprintf("%x", digest),
	})
	if err != nil {
		t.Fatalf("PutText(%q): %v", text, err)
	}
	return receipt
}

func fakeEvidenceContains(store *evidenceStoreFake, objectKey string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, found := store.objects[objectKey]
	return found
}

func allEvidenceKeys(store *evidenceStoreFake) []string {
	store.mu.Lock()
	defer store.mu.Unlock()
	keys := make([]string, 0, len(store.objects))
	for key := range store.objects {
		keys = append(keys, key)
	}
	return keys
}

type lifecyclePutBlockingStore struct {
	ingestiondomain.EvidenceStore
	written chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	puts    int
}

func newLifecyclePutBlockingStore(store ingestiondomain.EvidenceStore) *lifecyclePutBlockingStore {
	return &lifecyclePutBlockingStore{EvidenceStore: store, written: make(chan struct{}), release: make(chan struct{})}
}

func (store *lifecyclePutBlockingStore) PutText(ctx context.Context, object ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	receipt, err := store.EvidenceStore.PutText(ctx, object)
	if err != nil {
		return ingestiondomain.EvidenceReceipt{}, err
	}
	store.mu.Lock()
	store.puts++
	store.mu.Unlock()
	store.once.Do(func() { close(store.written) })
	<-store.release
	return receipt, nil
}

func (store *lifecyclePutBlockingStore) waitForPut(t *testing.T) {
	t.Helper()
	select {
	case <-store.written:
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not finish its first evidence PutText")
	}
}

func (store *lifecyclePutBlockingStore) releasePut() {
	select {
	case <-store.release:
	default:
		close(store.release)
	}
}

func (store *lifecyclePutBlockingStore) putCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.puts
}

func seedLifecycleReplayRun(t *testing.T, runtime *database.Runtime, sourceID int64, item sourcedomain.CapturedItem) int64 {
	t.Helper()
	now := time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC)
	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at, status)
VALUES ($1, $2, $3, $4, 'manual', $3, 'succeeded')
RETURNING id`, sourceID, strings.Repeat("b", 64), now, now.Add(time.Hour)).Scan(&runID); err != nil {
		t.Fatalf("create replay collection run: %v", err)
	}
	payload, err := json.Marshal(capturedPayload{Version: item.Version, SourceCode: item.SourceCode, ExternalID: item.ExternalID, ContentType: item.ContentType, Title: item.Title, Body: item.Body, Language: item.Language, URL: item.URL, Author: item.Author, PublishedAt: item.PublishedAt, ObservedAt: item.ObservedAt, Metrics: item.Metrics, RawPayloadDisposition: item.RawPayloadDisposition})
	if err != nil {
		t.Fatalf("marshal replay captured item: %v", err)
	}
	hash := sha256.Sum256(payload)
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version,
    captured_item, payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, 'captured', $10)`,
		runID, sourceID, item.SourceCode, item.ExternalID, item.ContentType, item.Version, string(payload), hex.EncodeToString(hash[:]), string(item.RawPayloadDisposition), item.ObservedAt); err != nil {
		t.Fatalf("insert replay captured item: %v", err)
	}
	return runID
}
