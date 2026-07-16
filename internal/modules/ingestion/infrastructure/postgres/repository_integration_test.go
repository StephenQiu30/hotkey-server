package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestContentRepositoryUpsertIsSourceIdempotentAndRaceSafe(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "race-safe")
	base := normalizedContent(sourceID, "source-retry", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))

	const callers = 6
	results := make(chan struct {
		content ingestiondomain.Content
		created bool
		err     error
	}, callers)
	var group sync.WaitGroup
	for index := range callers {
		candidate := base
		candidate.FetchedAt = base.FetchedAt.Add(time.Duration(index) * time.Minute)
		group.Add(1)
		go func(content ingestiondomain.NormalizedContent) {
			defer group.Done()
			stored, created, err := repository.Upsert(context.Background(), content, activeDecision())
			results <- struct {
				content ingestiondomain.Content
				created bool
				err     error
			}{stored, created, err}
		}(candidate)
	}
	group.Wait()
	close(results)

	var contentID int64
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("Upsert() error = %v", result.err)
		}
		if contentID == 0 {
			contentID = result.content.ID
		}
		if result.content.ID != contentID {
			t.Fatalf("Upsert() id = %d, want stable id %d", result.content.ID, contentID)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}

	var contents, snapshots int
	var version int64
	if err := runtime.SQL.QueryRow(`SELECT count(*), max(version) FROM contents WHERE source_connection_id = $1 AND external_id = $2`, sourceID, base.ExternalID).Scan(&contents, &version); err != nil {
		t.Fatalf("read idempotent content: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_metric_snapshots WHERE content_id = $1`, contentID).Scan(&snapshots); err != nil {
		t.Fatalf("read idempotent snapshots: %v", err)
	}
	if contents != 1 || version != callers || snapshots != callers {
		t.Fatalf("idempotent state = contents=%d version=%d snapshots=%d, want 1/%d/%d", contents, version, snapshots, callers, callers)
	}
}

func TestContentRepositoryPersistsStableSourceAuthorAndDuplicateMetadata(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "author-duplicate")
	firstInput := normalizedContent(sourceID, "first", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	first, created, err := repository.Upsert(context.Background(), firstInput, activeDecision())
	if err != nil || !created {
		t.Fatalf("Upsert(first) content/created/error = %#v / %t / %v", first, created, err)
	}

	secondInput := normalizedContent(sourceID, "duplicate", firstInput.FetchedAt.Add(time.Minute))
	secondInput.Author = firstInput.Author
	targetID := first.ID
	decision := ingestiondomain.DedupeDecision{
		Status: ingestiondomain.ContentStatusDuplicate, DuplicateOfID: &targetID,
		Reason: ingestiondomain.DedupeReasonExactHash, Version: ingestiondomain.DedupeVersionExactHash,
	}
	second, created, err := repository.Upsert(context.Background(), secondInput, decision)
	if err != nil || !created {
		t.Fatalf("Upsert(duplicate) content/created/error = %#v / %t / %v", second, created, err)
	}
	if second.Status != ingestiondomain.ContentStatusDuplicate || second.DuplicateOfID == nil || *second.DuplicateOfID != first.ID || second.DedupeReason != decision.Reason || second.DedupeVersion != decision.Version {
		t.Fatalf("duplicate content = %#v, want deterministic metadata", second)
	}
	if second.Author != first.Author {
		t.Fatalf("duplicate author = %#v, want stable source author %#v", second.Author, first.Author)
	}

	var authors int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_authors WHERE source_connection_id = $1 AND external_id = $2`, sourceID, first.Author.ExternalID).Scan(&authors); err != nil {
		t.Fatalf("count stable authors: %v", err)
	}
	if authors != 1 {
		t.Fatalf("source author count = %d, want 1", authors)
	}
}

func TestContentRepositoryRetryDoesNotCreateAnUnusedAuthor(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "retry-author")
	firstInput := normalizedContent(sourceID, "stable-author-item", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	first, created, err := repository.Upsert(context.Background(), firstInput, activeDecision())
	if err != nil || !created {
		t.Fatalf("Upsert(first) content/created/error = %#v / %t / %v", first, created, err)
	}

	retryInput := firstInput
	retryInput.FetchedAt = firstInput.FetchedAt.Add(time.Minute)
	retryInput.Author = ingestiondomain.NormalizedAuthor{ExternalID: strings.Repeat("d", 64), DisplayName: "Changed Author"}
	retried, created, err := repository.Upsert(context.Background(), retryInput, activeDecision())
	if err != nil || created {
		t.Fatalf("Upsert(retry) content/created/error = %#v / %t / %v", retried, created, err)
	}
	if retried.ID != first.ID || retried.Author != first.Author || retried.Version != first.Version+1 {
		t.Fatalf("retry result = %#v, want original author and versioned Content retry", retried)
	}

	var authors, unusedAuthors int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_authors WHERE source_connection_id = $1`, sourceID).Scan(&authors); err != nil {
		t.Fatalf("count source authors: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT count(*)
FROM source_authors AS author
LEFT JOIN contents AS content ON content.author_id = author.id
WHERE author.source_connection_id = $1 AND content.id IS NULL`, sourceID).Scan(&unusedAuthors); err != nil {
		t.Fatalf("count unused source authors: %v", err)
	}
	if authors != 1 || unusedAuthors != 0 {
		t.Fatalf("authors/unused authors = %d/%d, want 1/0", authors, unusedAuthors)
	}
}

func TestContentRepositoryNormalizesExternalIDForPersistenceAndDeletion(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "external-id-nfc")
	const nfdExternalID = "Cafe\u0301"
	const nfcExternalID = "Café"
	input := normalizedContent(sourceID, nfdExternalID, time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	stored, _, err := repository.Upsert(context.Background(), input, activeDecision())
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	deleted, changed, err := repository.MarkDeleted(context.Background(), sourceID, "  "+nfcExternalID+"  ")
	if err != nil || !changed {
		t.Fatalf("MarkDeleted(NFC equivalent) content/changed/error = %#v / %t / %v", deleted, changed, err)
	}
	if deleted.ID != stored.ID || deleted.ExternalID != nfcExternalID || deleted.Status != ingestiondomain.ContentStatusDeleted {
		t.Fatalf("deleted normalized content = %#v, want NFC tombstone for id %d", deleted, stored.ID)
	}
}

func TestContentRepositoryPreservesUnknownAndExplicitZeroMetrics(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "metrics")
	contentInput := normalizedContent(sourceID, "metrics", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	zero := int64(0)
	contentInput.Metrics = sourcedomain.SourceMetrics{LikeCount: &zero, ShareCount: sourcedomain.KnownMetric(4)}
	content, _, err := repository.Upsert(context.Background(), contentInput, activeDecision())
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	snapshotAt := contentInput.FetchedAt.Add(time.Minute)
	if err := repository.AppendMetricSnapshot(context.Background(), content.ID, snapshotAt, sourcedomain.SourceMetrics{ViewCount: &zero, CommentCount: sourcedomain.KnownMetric(8)}); err != nil {
		t.Fatalf("AppendMetricSnapshot() error = %v", err)
	}

	var currentView, currentLike, currentComment, currentShare sql.NullInt64
	if err := runtime.SQL.QueryRow(`SELECT view_count, like_count, comment_count, share_count FROM contents WHERE id = $1`, content.ID).Scan(&currentView, &currentLike, &currentComment, &currentShare); err != nil {
		t.Fatalf("read content metrics: %v", err)
	}
	if currentView.Valid || !currentLike.Valid || currentLike.Int64 != 0 || currentComment.Valid || !currentShare.Valid || currentShare.Int64 != 4 {
		t.Fatalf("current nullable metrics = %#v/%#v/%#v/%#v", currentView, currentLike, currentComment, currentShare)
	}

	var snapshotView, snapshotLike, snapshotComment, snapshotShare sql.NullInt64
	if err := runtime.SQL.QueryRow(`
SELECT view_count, like_count, comment_count, share_count
FROM content_metric_snapshots
WHERE content_id = $1 AND captured_at = $2`, content.ID, snapshotAt).Scan(&snapshotView, &snapshotLike, &snapshotComment, &snapshotShare); err != nil {
		t.Fatalf("read appended snapshot: %v", err)
	}
	if !snapshotView.Valid || snapshotView.Int64 != 0 || snapshotLike.Valid || !snapshotComment.Valid || snapshotComment.Int64 != 8 || snapshotShare.Valid {
		t.Fatalf("snapshot nullable metrics = %#v/%#v/%#v/%#v", snapshotView, snapshotLike, snapshotComment, snapshotShare)
	}
}

func TestContentRepositoryListsOnlyActiveContentWithPublishedCursor(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "cursor")
	base := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		externalID string
		published  time.Time
	}{
		{externalID: "middle", published: base.Add(time.Minute)},
		{externalID: "newest", published: base.Add(2 * time.Minute)},
		{externalID: "oldest", published: base},
	} {
		content := normalizedContent(sourceID, test.externalID, test.published)
		content.PublishedAt = test.published
		if _, _, err := repository.Upsert(context.Background(), content, activeDecision()); err != nil {
			t.Fatalf("Upsert(%s): %v", test.externalID, err)
		}
	}

	first, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("ListActive(first) page/error = %#v / %v, want two items and cursor", first, err)
	}
	second, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("ListActive(second) page/error = %#v / %v, want final item", second, err)
	}
	ids := map[int64]struct{}{}
	for _, content := range append(first.Items, second.Items...) {
		if content.Status != ingestiondomain.ContentStatusActive {
			t.Fatalf("listed status = %q, want active", content.Status)
		}
		if _, found := ids[content.ID]; found {
			t.Fatalf("cursor duplicated content id %d", content.ID)
		}
		ids[content.ID] = struct{}{}
	}
	if got := []string{first.Items[0].ExternalID, first.Items[1].ExternalID, second.Items[0].ExternalID}; strings.Join(got, ",") != "newest,middle,oldest" {
		t.Fatalf("published order = %v, want newest,middle,oldest", got)
	}
}

func TestContentRepositoryAssetsAreUniqueAndVersionConflictSafe(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "assets")
	first, _, err := repository.Upsert(context.Background(), normalizedContent(sourceID, "asset-first", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)), activeDecision())
	if err != nil {
		t.Fatalf("Upsert(first): %v", err)
	}
	second, _, err := repository.Upsert(context.Background(), normalizedContent(sourceID, "asset-second", time.Date(2026, time.July, 16, 9, 1, 0, 0, time.UTC)), activeDecision())
	if err != nil {
		t.Fatalf("Upsert(second): %v", err)
	}
	asset := contentAsset(first.ID, "evidence/v1/1/aa/"+strings.Repeat("a", 64)+".txt")
	if err := repository.CreateAsset(context.Background(), asset); err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	duplicate := asset
	duplicate.ContentID = second.ID
	if err := repository.CreateAsset(context.Background(), duplicate); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("CreateAsset(duplicate object key) error = %v, want conflict", err)
	}

	if _, err := runtime.SQL.Exec(`
CREATE OR REPLACE FUNCTION content_asset_status_conflict_test()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_sleep(0.1);
    RETURN NEW;
END;
$$;
CREATE TRIGGER content_asset_status_conflict_test
BEFORE UPDATE ON content_assets
FOR EACH ROW EXECUTE FUNCTION content_asset_status_conflict_test();`); err != nil {
		t.Fatalf("install asset conflict trigger: %v", err)
	}
	defer func() {
		_, _ = runtime.SQL.Exec(`DROP TRIGGER IF EXISTS content_asset_status_conflict_test ON content_assets; DROP FUNCTION IF EXISTS content_asset_status_conflict_test();`)
	}()

	start := make(chan struct{})
	errorsByUpdate := make(chan error, 2)
	var group sync.WaitGroup
	for _, status := range []ingestiondomain.AssetStatus{ingestiondomain.AssetStatusAvailable, ingestiondomain.AssetStatusMissing} {
		group.Add(1)
		go func(status ingestiondomain.AssetStatus) {
			defer group.Done()
			<-start
			errorsByUpdate <- repository.MarkAssetStatus(context.Background(), asset.ObjectKey, status)
		}(status)
	}
	close(start)
	group.Wait()
	close(errorsByUpdate)
	var successes, conflicts int
	for updateErr := range errorsByUpdate {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, sharedrepository.ErrConflict):
			conflicts++
		default:
			t.Fatalf("MarkAssetStatus() error = %v", updateErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("versioned status outcomes = %d success / %d conflicts, want 1 / 1", successes, conflicts)
	}
}

func TestContentRepositoryRejectsCredentialBearingAssetOriginalURL(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "asset-credentials")
	content, _, err := repository.Upsert(context.Background(), normalizedContent(sourceID, "asset-credential-content", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)), activeDecision())
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	for index, rawURL := range []string{
		"https://objects.example.test/evidence?api_key=opaque",
		"https://objects.example.test/evidence?X-AmZ-SiGnAtUrE=opaque",
		"https://objects.example.test/evidence?SiG=opaque",
		"https://objects.example.test/evidence#access_token=opaque",
	} {
		asset := contentAsset(content.ID, fmt.Sprintf("evidence/v1/credential/%d/%s.txt", index, strings.Repeat("a", 64)))
		asset.OriginalURL = rawURL
		if err := repository.CreateAsset(context.Background(), asset); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("CreateAsset(%q) error = %v, want invalid input", rawURL, err)
		}
	}
	var assets int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_assets WHERE content_id = $1`, content.ID).Scan(&assets); err != nil {
		t.Fatalf("count rejected assets: %v", err)
	}
	if assets != 0 {
		t.Fatalf("credential-bearing asset count = %d, want 0", assets)
	}
}

func TestContentRepositoryDeletedContentIsNotActiveAndUpsertReusesCallerTransaction(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "deleted")
	contentInput := normalizedContent(sourceID, "deleted-item", time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))

	rollback := errors.New("force rollback")
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		if _, _, err := repository.Upsert(ctx, contentInput, activeDecision()); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("WithinTransaction() error = %v, want rollback sentinel", err)
	}
	var rolledBack int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM contents WHERE source_connection_id = $1 AND external_id = $2`, sourceID, contentInput.ExternalID).Scan(&rolledBack); err != nil {
		t.Fatalf("count rolled-back content: %v", err)
	}
	if rolledBack != 0 {
		t.Fatalf("rolled-back content count = %d, want 0", rolledBack)
	}

	content, _, err := repository.Upsert(context.Background(), contentInput, activeDecision())
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	deleted, changed, err := repository.MarkDeleted(context.Background(), sourceID, contentInput.ExternalID)
	if err != nil || !changed {
		t.Fatalf("MarkDeleted() content/changed/error = %#v / %t / %v", deleted, changed, err)
	}
	if deleted.Status != ingestiondomain.ContentStatusDeleted || deleted.DeletedAt == nil || deleted.Version != content.Version+1 {
		t.Fatalf("deleted content = %#v, want versioned tombstone", deleted)
	}
	page, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("ListActive() items = %#v, want deleted content excluded", page.Items)
	}

	duplicateInput := normalizedContent(sourceID, "duplicate-deleted-item", contentInput.FetchedAt.Add(time.Minute))
	targetID := content.ID
	duplicate, _, err := repository.Upsert(context.Background(), duplicateInput, ingestiondomain.DedupeDecision{
		Status: ingestiondomain.ContentStatusDuplicate, DuplicateOfID: &targetID,
		Reason: ingestiondomain.DedupeReasonExactHash, Version: ingestiondomain.DedupeVersionExactHash,
	})
	if err != nil {
		t.Fatalf("Upsert(duplicate): %v", err)
	}
	duplicateDeleted, changed, err := repository.MarkDeleted(context.Background(), sourceID, duplicate.ExternalID)
	if err != nil || !changed {
		t.Fatalf("MarkDeleted(duplicate) content/changed/error = %#v / %t / %v", duplicateDeleted, changed, err)
	}
	if duplicateDeleted.DuplicateOfID != nil || duplicateDeleted.DedupeReason != "" || duplicateDeleted.DedupeVersion != "" {
		t.Fatalf("deleted duplicate metadata = %#v, want cleared non-active relationship", duplicateDeleted)
	}
}

func openContentRuntime(t *testing.T) *database.Runtime {
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

func createContentSource(t *testing.T, runtime *database.Runtime, suffix string) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, 'https://feeds.example.test/rss')
RETURNING id`, fmt.Sprintf("content-%s-%d", suffix, time.Now().UnixNano())).Scan(&sourceID); err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	return sourceID
}

func normalizedContent(sourceID int64, externalID string, observedAt time.Time) ingestiondomain.NormalizedContent {
	return ingestiondomain.NormalizedContent{
		SourceConnectionID: sourceID,
		ExternalID:         externalID,
		ContentType:        "article",
		Title:              "Stable content " + externalID,
		Excerpt:            "safe excerpt " + externalID,
		Body:               "body that must not enter contents " + externalID,
		CanonicalURL:       "https://example.test/content/" + externalID,
		Language:           "en",
		Author: ingestiondomain.NormalizedAuthor{
			ExternalID:  strings.Repeat("a", 63) + "b",
			DisplayName: "Stable Author",
		},
		PublishedAt: observedAt,
		FetchedAt:   observedAt,
		ContentHash: strings.Repeat("c", 64),
		Metrics: sourcedomain.SourceMetrics{
			ViewCount: sourcedomain.KnownMetric(12),
		},
	}
}

func activeDecision() ingestiondomain.DedupeDecision {
	return ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive}
}

func contentAsset(contentID int64, objectKey string) ingestiondomain.ContentAsset {
	return ingestiondomain.ContentAsset{
		ContentID:   contentID,
		AssetType:   "text",
		ObjectKey:   objectKey,
		OriginalURL: "https://example.test/content",
		MIMEType:    "text/plain; charset=utf-8",
		SHA256:      strings.Repeat("a", 64),
		SizeBytes:   12,
		CapturedAt:  time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC),
		Status:      ingestiondomain.AssetStatusPending,
	}
}
