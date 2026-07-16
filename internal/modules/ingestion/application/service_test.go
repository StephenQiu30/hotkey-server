package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestIngestRunPersistsEvidenceBindsContentAndContinuesAfterParseFailure(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	runID, sourceID := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("invalid", "unsupported", "Will fail", "must not upload"),
		capturedItem("body", "article", "Evidence title", "permitted evidence body"),
	})

	store := newEvidenceStoreFake()
	reader := transactionObservingReader{CapturedItemReader: newCapturedItemReader(t, runtime)}
	contents := transactionObservingContents{ContentRepository: ingestionpostgres.NewContentRepository(runtime)}
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: &reader, Contents: &contents, Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 10})
	if err != nil {
		t.Fatalf("IngestRun() error = %v", err)
	}
	if result.Processed != 2 || result.Bound != 1 || result.Failed != 1 || result.Uploaded != 1 {
		t.Fatalf("IngestRun() result = %#v, want 2 processed, 1 bound, 1 failed, 1 uploaded", result)
	}
	if store.puts != 1 {
		t.Fatalf("EvidenceStore.PutText calls = %d, want one permitted-body upload", store.puts)
	}
	if !reader.bindInTransaction || !contents.upsertInTransaction || !contents.assetInTransaction {
		t.Fatalf("per-item writes did not all receive the Runtime transaction: reader=%t upsert=%t asset=%t", reader.bindInTransaction, contents.upsertInTransaction, contents.assetInTransaction)
	}
	if reader.bindTransaction == nil || reader.bindTransaction != contents.upsertTransaction || reader.bindTransaction != contents.assetTransaction {
		t.Fatal("Content upsert, asset write, and Source bind did not reuse one SQL transaction")
	}

	var succeeded, failed, assets int
	var failedCode string
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id = $1 AND ingestion_status = 'succeeded'`, runID).Scan(&succeeded); err != nil {
		t.Fatalf("count succeeded capture: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id = $1 AND ingestion_status = 'failed'`, runID).Scan(&failed); err != nil {
		t.Fatalf("count failed capture: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT ingestion_error_code FROM collection_run_items WHERE run_id = $1 AND external_id = 'invalid'`, runID).Scan(&failedCode); err != nil {
		t.Fatalf("read controlled failure code: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_assets`).Scan(&assets); err != nil {
		t.Fatalf("count persisted evidence assets: %v", err)
	}
	if succeeded != 1 || failed != 1 || failedCode != "invalid_content_type" || assets != 1 {
		t.Fatalf("persisted ingestion state = succeeded=%d failed=%d code=%q assets=%d", succeeded, failed, failedCode, assets)
	}

	replay, err := service.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 10})
	if err != nil {
		t.Fatalf("IngestRun(replay) error = %v", err)
	}
	if replay.Processed != 0 || store.puts != 1 {
		t.Fatalf("replay result/uploads = %#v/%d, want no reprocessed capture or duplicate object put", replay, store.puts)
	}
	if sourceID <= 0 {
		t.Fatal("test fixture source id was not assigned")
	}
}

func TestIngestRunDoesNotUploadWhenCaptureBodyWasNotPermitted(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	runID, _ := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("title-only", "article", "Persist this title", ""),
	})
	store := newEvidenceStoreFake()
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: newCapturedItemReader(t, runtime), Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 1})
	if err != nil {
		t.Fatalf("IngestRun() error = %v", err)
	}
	if result.Bound != 1 || result.Uploaded != 0 || store.puts != 0 {
		t.Fatalf("title-only ingestion result/uploads = %#v/%d, want content binding without object upload", result, store.puts)
	}
	var assets int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_assets`).Scan(&assets); err != nil {
		t.Fatalf("count assets: %v", err)
	}
	if assets != 0 {
		t.Fatalf("content assets = %d, want no body evidence asset", assets)
	}
}

func TestIngestRunReusesOneEvidenceAssetForSameSourceBody(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	runID, _ := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("same-body-first", "article", "Same body", "one deterministic evidence object"),
		capturedItem("same-body-second", "article", "Same body", "one deterministic evidence object"),
	})
	store := newEvidenceStoreFake()
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: newCapturedItemReader(t, runtime), Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.IngestRun(context.Background(), ingestionapplication.IngestRunInput{RunID: runID, Limit: 2})
	if err != nil {
		t.Fatalf("IngestRun() error = %v", err)
	}
	if result.Bound != 2 || result.Failed != 0 || store.puts != 2 {
		t.Fatalf("same-SHA result/puts = %#v/%d, want both captures bound through idempotent evidence puts", result, store.puts)
	}
	var assets, succeeded int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_assets`).Scan(&assets); err != nil {
		t.Fatalf("count shared evidence assets: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE ingestion_status = 'succeeded'`).Scan(&succeeded); err != nil {
		t.Fatalf("count shared evidence bindings: %v", err)
	}
	if assets != 1 || succeeded != 2 {
		t.Fatalf("shared evidence state = assets=%d bindings=%d, want 1/2", assets, succeeded)
	}
}

func TestIngestRunCompletesWithSingleConnectionPool(t *testing.T) {
	runtime := openSingleConnectionIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	runID, _ := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("single-pool", "article", "Single pool", "evidence must not deadlock"),
	})
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: newCapturedItemReader(t, runtime), Contents: ingestionpostgres.NewContentRepository(runtime), Evidence: newEvidenceStoreFake(),
	})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := service.IngestRun(ctx, ingestionapplication.IngestRunInput{RunID: runID, Limit: 1})
	if err != nil || result.Bound != 1 || result.Failed != 0 {
		t.Fatalf("IngestRun(single connection pool) result/error = %#v / %v, want successful bounded completion", result, err)
	}
}

type transactionObservingReader struct {
	sourcedomain.CapturedItemReader
	bindInTransaction bool
	bindTransaction   any
}

func (reader *transactionObservingReader) BindContent(ctx context.Context, binding sourcedomain.CapturedContentBinding) error {
	transaction, found := database.TransactionFromContext(ctx)
	reader.bindInTransaction = found
	reader.bindTransaction = transaction.SQL
	return reader.CapturedItemReader.BindContent(ctx, binding)
}

type transactionObservingContents struct {
	ingestiondomain.ContentRepository
	upsertInTransaction bool
	assetInTransaction  bool
	upsertTransaction   any
	assetTransaction    any
}

func (repository *transactionObservingContents) Upsert(ctx context.Context, content ingestiondomain.NormalizedContent, decision ingestiondomain.DedupeDecision) (ingestiondomain.Content, bool, error) {
	transaction, found := database.TransactionFromContext(ctx)
	repository.upsertInTransaction = found
	repository.upsertTransaction = transaction.SQL
	return repository.ContentRepository.Upsert(ctx, content, decision)
}

func (repository *transactionObservingContents) CreateAsset(ctx context.Context, asset ingestiondomain.ContentAsset) error {
	transaction, found := database.TransactionFromContext(ctx)
	repository.assetInTransaction = found
	repository.assetTransaction = transaction.SQL
	return repository.ContentRepository.CreateAsset(ctx, asset)
}

type evidenceStoreFake struct {
	mu      sync.Mutex
	puts    int
	objects map[string]ingestiondomain.EvidenceReceipt
}

func newEvidenceStoreFake() *evidenceStoreFake {
	return &evidenceStoreFake{objects: make(map[string]ingestiondomain.EvidenceReceipt)}
}

func (store *evidenceStoreFake) PutText(_ context.Context, object ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.puts++
	receipt := ingestiondomain.EvidenceReceipt{ObjectKey: object.ObjectKey, SHA256: object.SHA256, SizeBytes: int64(len(object.Text))}
	store.objects[object.ObjectKey] = receipt
	return receipt, nil
}

func (store *evidenceStoreFake) Delete(_ context.Context, objectKey string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.objects, objectKey)
	return nil
}

func (store *evidenceStoreFake) ListPrefix(_ context.Context, prefix string) ([]ingestiondomain.EvidenceReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	receipts := make([]ingestiondomain.EvidenceReceipt, 0)
	for key, receipt := range store.objects {
		if strings.HasPrefix(key, prefix) {
			receipts = append(receipts, receipt)
		}
	}
	return receipts, nil
}

func openIngestionRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}

func openSingleConnectionIngestionRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dsn, err := url.Parse(postgresfixture.New(t))
	if err != nil {
		t.Fatalf("parse PostgreSQL fixture DSN: %v", err)
	}
	query := dsn.Query()
	query.Set("pool_max_conns", "1")
	dsn.RawQuery = query.Encode()
	runtime, err := database.Open(ctx, dsn.String())
	if err != nil {
		t.Fatalf("database.Open(single connection): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(single connection): %v", err)
	}
	return runtime
}

func newCapturedItemReader(t *testing.T, runtime *database.Runtime) *sourceapplication.CapturedItemReader {
	t.Helper()
	reader, err := sourceapplication.NewCapturedItemReader(sourceapplication.CapturedItemReaderDependencies{Runs: sourcepostgres.NewCollectionRepository(runtime)})
	if err != nil {
		t.Fatalf("NewCapturedItemReader(): %v", err)
	}
	return reader
}

func seedCapturedRun(t *testing.T, runtime *database.Runtime, items []sourcedomain.CapturedItem) (int64, int64) {
	t.Helper()
	const signature = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	now := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	var sourceID, runID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', $1, 'https://feeds.example.test/ingestion') RETURNING id`, fmt.Sprintf("ingestion-service-%d", time.Now().UnixNano())).Scan(&sourceID); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at, status)
VALUES ($1, $2, $3, $4, 'manual', $3, 'succeeded')
RETURNING id`, sourceID, signature, now, now.Add(time.Hour)).Scan(&runID); err != nil {
		t.Fatalf("create collection run: %v", err)
	}
	for _, item := range items {
		payload, err := json.Marshal(capturedPayload{Version: item.Version, SourceCode: item.SourceCode, ExternalID: item.ExternalID, ContentType: item.ContentType, Title: item.Title, Body: item.Body, Language: item.Language, URL: item.URL, Author: item.Author, PublishedAt: item.PublishedAt, ObservedAt: item.ObservedAt, Metrics: item.Metrics, RawPayloadDisposition: item.RawPayloadDisposition})
		if err != nil {
			t.Fatalf("marshal captured item: %v", err)
		}
		hash := sha256.Sum256(payload)
		if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version,
    captured_item, payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, 'captured', $10)`,
			runID, sourceID, item.SourceCode, item.ExternalID, item.ContentType, item.Version, string(payload), hex.EncodeToString(hash[:]), string(item.RawPayloadDisposition), item.ObservedAt); err != nil {
			t.Fatalf("insert captured item %q: %v", item.ExternalID, err)
		}
	}
	return runID, sourceID
}

type capturedPayload struct {
	Version               string                             `json:"version"`
	SourceCode            string                             `json:"source_code"`
	ExternalID            string                             `json:"external_id"`
	ContentType           string                             `json:"content_type"`
	Title                 string                             `json:"title"`
	Body                  string                             `json:"body,omitempty"`
	Language              string                             `json:"language,omitempty"`
	URL                   string                             `json:"url,omitempty"`
	Author                string                             `json:"author,omitempty"`
	PublishedAt           *time.Time                         `json:"published_at,omitempty"`
	ObservedAt            time.Time                          `json:"observed_at"`
	Metrics               sourcedomain.SourceMetrics         `json:"metrics"`
	RawPayloadDisposition sourcedomain.RawPayloadDisposition `json:"raw_payload_disposition"`
}

func capturedItem(externalID, contentType, title, body string) sourcedomain.CapturedItem {
	observedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	return sourcedomain.CapturedItem{
		Version: sourcedomain.CapturedItemVersionV2, SourceCode: "rss", ExternalID: externalID, ContentType: contentType,
		Title: title, Body: body, URL: "https://example.test/articles/" + externalID, ObservedAt: observedAt,
		RawPayloadDisposition: sourcedomain.RawPayloadDiscarded,
	}
}
