package application_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func TestExpireBeforeExcludesOnlyExpiredActiveContentFromCursor(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createLifecycleSource(t, runtime, "expire")
	cutoff := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	old := lifecycleContent(sourceID, "expired", cutoff.Add(-time.Hour))
	current := lifecycleContent(sourceID, "current", cutoff.Add(time.Hour))
	if _, _, err := repository.Upsert(context.Background(), old, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive}); err != nil {
		t.Fatalf("Upsert(old) error = %v", err)
	}
	if _, _, err := repository.Upsert(context.Background(), current, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive}); err != nil {
		t.Fatalf("Upsert(current) error = %v", err)
	}
	service := newLifecycleService(t, runtime, newEvidenceStoreFake())
	expired, err := service.ExpireBefore(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("ExpireBefore() error = %v", err)
	}
	if expired != 1 {
		t.Fatalf("ExpireBefore() expired = %d, want one active content", expired)
	}
	page, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ExternalID != current.ExternalID {
		t.Fatalf("ListActive() after expiration = %#v, want only current content", page.Items)
	}
	var oldStatus string
	if err := runtime.SQL.QueryRow(`SELECT content_status FROM contents WHERE source_connection_id = $1 AND external_id = $2`, sourceID, old.ExternalID).Scan(&oldStatus); err != nil {
		t.Fatalf("read expired content: %v", err)
	}
	if oldStatus != string(ingestiondomain.ContentStatusExpired) {
		t.Fatalf("expired content status = %q, want %q", oldStatus, ingestiondomain.ContentStatusExpired)
	}
}

func createLifecycleSource(t *testing.T, runtime *database.Runtime, name string) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, 'https://feeds.example.test/lifecycle')
RETURNING id`, fmt.Sprintf("lifecycle-%s-%d", name, time.Now().UnixNano())).Scan(&sourceID); err != nil {
		t.Fatalf("create lifecycle source: %v", err)
	}
	return sourceID
}

func lifecycleContent(sourceID int64, externalID string, observedAt time.Time) ingestiondomain.NormalizedContent {
	return ingestiondomain.NormalizedContent{
		SourceConnectionID: sourceID,
		ExternalID:         externalID,
		ContentType:        "article",
		Title:              "Lifecycle " + externalID,
		Excerpt:            "Safe lifecycle excerpt " + externalID,
		CanonicalURL:       "https://example.test/lifecycle/" + externalID,
		Language:           "en",
		PublishedAt:        observedAt,
		FetchedAt:          observedAt,
		ContentHash:        strings.Repeat("a", 64),
	}
}

func TestArchiveMarkdownOrphanReconciliation(t *testing.T) {
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createLifecycleSource(t, runtime, "reconcile")
	content := lifecycleContent(sourceID, "known-evidence", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	stored, _, err := repository.Upsert(context.Background(), content, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	store := newEvidenceStoreFake()
	known := deterministicEvidence(t, store, sourceID, "known evidence")
	if err := repository.CreateAsset(context.Background(), ingestiondomain.ContentAsset{
		ContentID: stored.ID, AssetType: "text", ObjectKey: known.ObjectKey, OriginalURL: content.CanonicalURL,
		MIMEType: archivedMarkdownMIME, SHA256: known.SHA256, SizeBytes: known.SizeBytes,
		CapturedAt: content.FetchedAt, Status: ingestiondomain.AssetStatusAvailable,
	}); err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	orphan := deterministicEvidence(t, store, sourceID, "orphan evidence")
	service := newLifecycleService(t, runtime, store)
	deleted, err := service.ReconcileObjects(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("ReconcileObjects() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("ReconcileObjects() deleted = %d, want one orphan", deleted)
	}
	if !fakeEvidenceContains(store, known.ObjectKey) || fakeEvidenceContains(store, orphan.ObjectKey) {
		t.Fatalf("reconciled evidence keys = %#v, want only available known asset", allEvidenceKeys(store))
	}
}
