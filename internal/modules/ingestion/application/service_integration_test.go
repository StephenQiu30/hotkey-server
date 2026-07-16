//go:build integration

package application_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionminio "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/minio"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestIngestRunMinIOPostgresRollbackDeletesObject(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	body := fmt.Sprintf("rollback evidence %d", time.Now().UnixNano())
	runID, sourceID := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("rollback-delete", "article", "Rollback evidence", body),
	})
	store, client, cfg := integrationEvidenceStore(t)
	cleanupEvidencePrefix(t, store, sourceID)
	t.Cleanup(func() { cleanupEvidencePrefix(t, store, sourceID) })

	reader := failingBindReader{CapturedItemReader: newCapturedItemReader(t, runtime), err: errors.New("inject source bind failure")}
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: &reader, Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	result, err := service.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 1})
	if err != nil {
		t.Fatalf("IngestRun(): %v", err)
	}
	if result.Processed != 1 || result.Bound != 0 || result.Failed != 1 || result.Uploaded != 0 {
		t.Fatalf("IngestRun() result = %#v, want one classified rollback", result)
	}
	if reader.bindCalls != 1 {
		t.Fatalf("injected Source bind calls = %d, want one call inside transaction", reader.bindCalls)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	assertNoEvidenceObjects(t, context.Background(), client, cfg, store, sourceID, ingestionminio.EvidenceObjectKey(sourceID, digest))
	assertIngestionRollback(t, runtime, runID, 0)
}

func TestIngestRunMinIOPostgresReconcileDeletesOrphan(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	runID, sourceID := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("rollback-orphan", "article", "Orphan evidence", fmt.Sprintf("orphan evidence %d", time.Now().UnixNano())),
	})
	store, client, cfg := integrationEvidenceStore(t)
	cleanupEvidencePrefix(t, store, sourceID)
	t.Cleanup(func() { cleanupEvidencePrefix(t, store, sourceID) })

	failingDelete := &failFirstDeleteStore{EvidenceStore: store}
	reader := failingBindReader{CapturedItemReader: newCapturedItemReader(t, runtime), err: errors.New("inject source bind failure")}
	first, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: &reader, Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: failingDelete,
	})
	if err != nil {
		t.Fatalf("NewService(first): %v", err)
	}
	result, err := first.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 1})
	if err != nil {
		t.Fatalf("IngestRun(): %v", err)
	}
	if result.Failed != 1 || failingDelete.deleteCalls != 1 {
		t.Fatalf("rollback result/delete calls = %#v/%d, want one classified failure and first compensation delete failure", result, failingDelete.deleteCalls)
	}
	assertIngestionRollback(t, runtime, runID, 0)

	orphan := onlyEvidenceReceipt(t, store, sourceID)
	if _, err := client.StatObject(context.Background(), cfg.Bucket, orphan.ObjectKey, miniosdk.StatObjectOptions{}); err != nil {
		t.Fatalf("Head orphan before reconciliation: %v", err)
	}
	known := createKnownEvidenceAsset(t, runtime, store, sourceID)

	// Reconciliation must recover a durable orphan after a Service restart. It
	// may only remove the unreferenced object and must retain the known asset.
	second, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: newCapturedItemReader(t, runtime), Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewService(second): %v", err)
	}
	deleted, err := second.ReconcileObjects(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("ReconcileObjects(): %v", err)
	}
	if deleted != 1 {
		t.Fatalf("ReconcileObjects() deleted = %d, want one unreferenced orphan", deleted)
	}
	if _, err := client.StatObject(context.Background(), cfg.Bucket, orphan.ObjectKey, miniosdk.StatObjectOptions{}); err == nil {
		t.Fatal("Head orphan after reconciliation error = nil, want removed object")
	}
	if _, err := client.StatObject(context.Background(), cfg.Bucket, known.ObjectKey, miniosdk.StatObjectOptions{}); err != nil {
		t.Fatalf("Head known evidence after reconciliation: %v", err)
	}
	receipts, err := store.ListPrefix(context.Background(), fmt.Sprintf("evidence/v1/%d/", sourceID))
	if err != nil {
		t.Fatalf("ListPrefix after reconciliation: %v", err)
	}
	if len(receipts) != 1 || receipts[0].ObjectKey != known.ObjectKey {
		t.Fatalf("ListPrefix after reconciliation = %#v, want only known asset %#v", receipts, known)
	}
	var assets int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_assets`).Scan(&assets); err != nil {
		t.Fatalf("count known DB assets: %v", err)
	}
	if assets != 1 {
		t.Fatalf("content assets after reconciliation = %d, want preserved known asset", assets)
	}
}

func TestIngestRunMinIOPostgresRePutsEvidenceDeletedBeforeAssetTransaction(t *testing.T) {
	runtimeA := openIngestionRuntime(t)
	defer func() { _ = runtimeA.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runtimeB, err := database.Open(ctx, runtimeA.Pool.Config().ConnString())
	if err != nil {
		t.Fatalf("open independent runtime: %v", err)
	}
	defer func() { _ = runtimeB.Close() }()

	body := fmt.Sprintf("in-flight evidence %d", time.Now().UnixNano())
	runID, sourceID := seedCapturedRun(t, runtimeA, []sourcedomain.CapturedItem{
		capturedItem("in-flight", "article", "In-flight evidence", body),
	})
	store, client, cfg := integrationEvidenceStore(t)
	cleanupEvidencePrefix(t, store, sourceID)
	t.Cleanup(func() { cleanupEvidencePrefix(t, store, sourceID) })

	blockingStore := newBlockingAfterPutStore(store)
	defer blockingStore.releasePut()
	serviceA, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtimeA, Captures: newCapturedItemReader(t, runtimeA), Contents: ingestionpostgres.NewContentRepository(runtimeA), Evidence: blockingStore,
	})
	if err != nil {
		t.Fatalf("NewService(A): %v", err)
	}
	serviceB, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtimeB, Captures: newCapturedItemReader(t, runtimeB), Contents: ingestionpostgres.NewContentRepository(runtimeB), Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewService(B): %v", err)
	}

	ingestResult := make(chan struct {
		result ingestionapplication.IngestRunResult
		err    error
	}, 1)
	go func() {
		result, err := serviceA.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 1})
		ingestResult <- struct {
			result ingestionapplication.IngestRunResult
			err    error
		}{result, err}
	}()
	blockingStore.waitWritten(t)

	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	objectKey := ingestionminio.EvidenceObjectKey(sourceID, digest)
	if _, err := client.StatObject(context.Background(), cfg.Bucket, objectKey, miniosdk.StatObjectOptions{}); err != nil {
		t.Fatalf("Head in-flight object before DB reference: %v", err)
	}

	reconcileResult := make(chan struct {
		deleted int
		err     error
	}, 1)
	go func() {
		deleted, err := serviceB.ReconcileObjects(context.Background(), sourceID)
		reconcileResult <- struct {
			deleted int
			err     error
		}{deleted, err}
	}()
	select {
	case result := <-reconcileResult:
		if result.err != nil || result.deleted != 1 {
			t.Fatalf("ReconcileObjects(B) before asset transaction result/error = %d / %v, want one unreferenced pre-commit object deleted", result.deleted, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReconcileObjects(B) did not delete the pre-transaction object")
	}

	blockingStore.releasePut()
	select {
	case result := <-ingestResult:
		if result.err != nil || result.result.Bound != 1 || result.result.Failed != 0 {
			t.Fatalf("IngestRun(A) result/error = %#v / %v, want re-put then committed binding", result.result, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("IngestRun(A) did not commit after releasing evidence PutText")
	}
	if blockingStore.putCount() != 2 {
		t.Fatalf("evidence PutText calls = %d, want first object write plus outside-transaction re-put", blockingStore.putCount())
	}
	if _, err := client.StatObject(context.Background(), cfg.Bucket, objectKey, miniosdk.StatObjectOptions{}); err != nil {
		t.Fatalf("Head committed object after reconciliation: %v", err)
	}
	var assets, succeeded int
	if err := runtimeA.SQL.QueryRow(`SELECT count(*) FROM content_assets WHERE object_key = $1`, objectKey).Scan(&assets); err != nil {
		t.Fatalf("read committed asset: %v", err)
	}
	if err := runtimeA.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id = $1 AND ingestion_status = 'succeeded'`, runID).Scan(&succeeded); err != nil {
		t.Fatalf("read committed Source binding: %v", err)
	}
	if assets != 1 || succeeded != 1 {
		t.Fatalf("committed state after interleaving = assets=%d bindings=%d, want 1/1", assets, succeeded)
	}
}

type failingBindReader struct {
	sourcedomain.CapturedItemReader
	err       error
	bindCalls int
}

func (reader *failingBindReader) BindContent(context.Context, sourcedomain.CapturedContentBinding) error {
	reader.bindCalls++
	return reader.err
}

type failFirstDeleteStore struct {
	ingestiondomain.EvidenceStore
	mu          sync.Mutex
	deleteCalls int
}

type blockingAfterPutStore struct {
	ingestiondomain.EvidenceStore
	written chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	puts    int
}

func newBlockingAfterPutStore(store ingestiondomain.EvidenceStore) *blockingAfterPutStore {
	return &blockingAfterPutStore{EvidenceStore: store, written: make(chan struct{}), release: make(chan struct{})}
}

func (store *blockingAfterPutStore) PutText(ctx context.Context, object ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	receipt, err := store.EvidenceStore.PutText(ctx, object)
	if err != nil {
		return ingestiondomain.EvidenceReceipt{}, err
	}
	store.mu.Lock()
	store.puts++
	store.mu.Unlock()
	store.once.Do(func() { close(store.written) })
	select {
	case <-store.release:
		return receipt, nil
	case <-ctx.Done():
		return ingestiondomain.EvidenceReceipt{}, ctx.Err()
	}
}

func (store *blockingAfterPutStore) putCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.puts
}

func (store *blockingAfterPutStore) waitWritten(t *testing.T) {
	t.Helper()
	select {
	case <-store.written:
	case <-time.After(5 * time.Second):
		t.Fatal("ingestion did not write the object before its asset transaction")
	}
}

func (store *blockingAfterPutStore) releasePut() {
	store.once.Do(func() {})
	select {
	case <-store.release:
	default:
		close(store.release)
	}
}

func (store *failFirstDeleteStore) Delete(ctx context.Context, objectKey string) error {
	store.mu.Lock()
	store.deleteCalls++
	fail := store.deleteCalls == 1
	store.mu.Unlock()
	if fail {
		return errors.New("injected first evidence delete failure")
	}
	return store.EvidenceStore.Delete(ctx, objectKey)
}

func integrationEvidenceStore(t *testing.T) (*ingestionminio.Store, *miniosdk.Client, config.MinIOConfig) {
	t.Helper()
	cfg := config.MinIOConfig{
		Endpoint: os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT"), AccessKey: os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY"),
		SecretKey: os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY"), Bucket: os.Getenv("HOTKEY_TEST_MINIO_BUCKET"),
	}
	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatalf("integration MinIO configuration is required: %v", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL, Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("create integration MinIO client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.MakeBucket(ctx, cfg.Bucket, miniosdk.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		response := miniosdk.ToErrorResponse(err)
		if response.Code != "BucketAlreadyOwnedByYou" && response.Code != "BucketAlreadyExists" {
			t.Fatalf("MakeBucket(%q): %v", cfg.Bucket, err)
		}
	}
	store, err := ingestionminio.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore(): %v", err)
	}
	return store, client, cfg
}

func createKnownEvidenceAsset(t *testing.T, runtime *database.Runtime, store ingestiondomain.EvidenceStore, sourceID int64) ingestiondomain.EvidenceReceipt {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, time.July, 16, 10, 0, 0, 0, time.UTC)
	knownBody := fmt.Sprintf("known evidence %d", time.Now().UnixNano())
	knownHash := fmt.Sprintf("%x", sha256.Sum256([]byte(knownBody)))
	content := ingestiondomain.NormalizedContent{
		SourceConnectionID: sourceID, ExternalID: "known-evidence", ContentType: "article", Title: "Known evidence", Excerpt: "known evidence",
		CanonicalURL: "https://example.test/known-evidence", Language: "en", PublishedAt: now, FetchedAt: now,
		ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte("known evidence content"))),
	}
	repository := ingestionpostgres.NewContentRepository(runtime)
	stored, _, err := repository.Upsert(ctx, content, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive})
	if err != nil {
		t.Fatalf("upsert known content: %v", err)
	}
	object := ingestiondomain.EvidenceObject{
		SourceConnectionID: sourceID, ObjectKey: ingestionminio.EvidenceObjectKey(sourceID, knownHash), Text: knownBody, SHA256: knownHash,
	}
	receipt, err := store.PutText(ctx, object)
	if err != nil {
		t.Fatalf("PutText known evidence: %v", err)
	}
	if err := repository.CreateAsset(ctx, ingestiondomain.ContentAsset{
		ContentID: stored.ID, AssetType: "text", ObjectKey: receipt.ObjectKey, OriginalURL: content.CanonicalURL,
		MIMEType: "text/plain; charset=utf-8", SHA256: receipt.SHA256, SizeBytes: receipt.SizeBytes, CapturedAt: now, Status: ingestiondomain.AssetStatusAvailable,
	}); err != nil {
		t.Fatalf("CreateAsset known evidence: %v", err)
	}
	return receipt
}

func onlyEvidenceReceipt(t *testing.T, store ingestiondomain.EvidenceStore, sourceID int64) ingestiondomain.EvidenceReceipt {
	t.Helper()
	receipts, err := store.ListPrefix(context.Background(), fmt.Sprintf("evidence/v1/%d/", sourceID))
	if err != nil {
		t.Fatalf("ListPrefix(): %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("ListPrefix() = %#v, want one evidence object", receipts)
	}
	return receipts[0]
}

func cleanupEvidencePrefix(t *testing.T, store ingestiondomain.EvidenceStore, sourceID int64) {
	t.Helper()
	receipts, err := store.ListPrefix(context.Background(), fmt.Sprintf("evidence/v1/%d/", sourceID))
	if err != nil {
		t.Fatalf("ListPrefix cleanup: %v", err)
	}
	for _, receipt := range receipts {
		if err := store.Delete(context.Background(), receipt.ObjectKey); err != nil {
			t.Fatalf("Delete cleanup %q: %v", receipt.ObjectKey, err)
		}
	}
}

func assertNoEvidenceObjects(t *testing.T, ctx context.Context, client *miniosdk.Client, cfg config.MinIOConfig, store ingestiondomain.EvidenceStore, sourceID int64, absentKey string) {
	t.Helper()
	receipts, err := store.ListPrefix(ctx, fmt.Sprintf("evidence/v1/%d/", sourceID))
	if err != nil {
		t.Fatalf("ListPrefix(): %v", err)
	}
	if len(receipts) != 0 {
		keys := make([]string, 0, len(receipts))
		for _, receipt := range receipts {
			keys = append(keys, receipt.ObjectKey)
		}
		sort.Strings(keys)
		t.Fatalf("ListPrefix() keys = %v, want no rolled-back evidence object", keys)
	}
	if _, err := client.StatObject(ctx, cfg.Bucket, absentKey, miniosdk.StatObjectOptions{}); err == nil {
		t.Fatalf("Head rolled-back object %q error = nil, want object absence", absentKey)
	}
}

func assertIngestionRollback(t *testing.T, runtime *database.Runtime, runID int64, wantAssets int) {
	t.Helper()
	var contents, assets int
	var status, failureCode string
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM contents`).Scan(&contents); err != nil {
		t.Fatalf("count rolled-back contents: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_assets`).Scan(&assets); err != nil {
		t.Fatalf("count rolled-back assets: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT ingestion_status, ingestion_error_code FROM collection_run_items WHERE run_id = $1`, runID).Scan(&status, &failureCode); err != nil {
		t.Fatalf("read classified capture: %v", err)
	}
	if contents != 0 || assets != wantAssets || status != "failed" || failureCode != "ingestion_failed" {
		t.Fatalf("rollback state = contents=%d assets=%d status=%q code=%q, want 0/%d/failed/ingestion_failed", contents, assets, status, failureCode, wantAssets)
	}
}
