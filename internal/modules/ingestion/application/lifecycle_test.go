package application_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"

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
