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

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
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
	if _, err := repository.GetActive(context.Background(), second.ID); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("GetActive(duplicate) error = %v, want not found before downstream Event processing", err)
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
	const nfcExternalID = "Café"
	const nfdExternalID = "Cafe\u0301"
	input := normalizedContent(sourceID, nfcExternalID, time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	stored, _, err := repository.Upsert(context.Background(), input, activeDecision())
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	deleted, changed, err := repository.MarkDeleted(context.Background(), sourceID, "  "+nfdExternalID+"  ")
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
	if !currentView.Valid || currentView.Int64 != 0 || currentLike.Valid || !currentComment.Valid || currentComment.Int64 != 8 || currentShare.Valid {
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

func TestContentRepositoryPersistsUnknownPublishedAtAsNull(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentSource(t, runtime, "unknown-published-at")
	observedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	input := normalizedContent(sourceID, "unknown-published-at", observedAt)
	input.PublishedAt = time.Time{}

	stored, created, err := repository.Upsert(context.Background(), input, activeDecision())
	if err != nil || !created {
		t.Fatalf("Upsert() content/created/error = %#v / %t / %v", stored, created, err)
	}
	if !stored.PublishedAt.IsZero() || !stored.FetchedAt.Equal(observedAt) {
		t.Fatalf("stored times = published:%v fetched:%v, want unknown publication and preserved observation", stored.PublishedAt, stored.FetchedAt)
	}
	var publishedAt sql.NullTime
	if err := runtime.SQL.QueryRow(`SELECT published_at FROM contents WHERE id = $1`, stored.ID).Scan(&publishedAt); err != nil {
		t.Fatalf("read nullable publication time: %v", err)
	}
	if publishedAt.Valid {
		t.Fatalf("published_at = %v, want SQL NULL", publishedAt.Time)
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
		{externalID: "unknown-first", published: time.Time{}},
		{externalID: "unknown-second", published: time.Time{}},
	} {
		content := normalizedContent(sourceID, test.externalID, base.Add(3*time.Minute))
		content.PublishedAt = test.published
		if test.externalID == "newest" {
			content.Metrics.LikeCount = sourcedomain.KnownMetric(2_000)
		}
		if _, _, err := repository.Upsert(context.Background(), content, activeDecision()); err != nil {
			t.Fatalf("Upsert(%s): %v", test.externalID, err)
		}
	}

	first, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 2, IncludeSummary: true})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("ListActive(first) page/error = %#v / %v, want two items and cursor", first, err)
	}
	if first.Summary == nil || first.Summary.Total != 5 || first.Summary.Today != 0 || first.Summary.Urgent != 1 {
		t.Fatalf("ListActive(first) summary = %#v, want total=5 today=0 urgent=1", first.Summary)
	}
	second, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 2 || second.NextCursor == "" {
		t.Fatalf("ListActive(second) page/error = %#v / %v, want two items and unknown-time cursor", second, err)
	}
	third, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 2, Cursor: second.NextCursor})
	if err != nil || len(third.Items) != 1 || third.NextCursor != "" {
		t.Fatalf("ListActive(third) page/error = %#v / %v, want one final unknown-time item", third, err)
	}
	ids := map[int64]struct{}{}
	for _, content := range append(append(first.Items, second.Items...), third.Items...) {
		if content.Status != ingestiondomain.ContentStatusActive {
			t.Fatalf("listed status = %q, want active", content.Status)
		}
		if _, found := ids[content.ID]; found {
			t.Fatalf("cursor duplicated content id %d", content.ID)
		}
		ids[content.ID] = struct{}{}
	}
	if got := []string{first.Items[0].ExternalID, first.Items[1].ExternalID, second.Items[0].ExternalID, second.Items[1].ExternalID, third.Items[0].ExternalID}; strings.Join(got, ",") != "newest,middle,oldest,unknown-second,unknown-first" {
		t.Fatalf("published order = %v, want known times then stable unknown-time IDs", got)
	}
}

func TestContentRepositorySearchFiltersLatestMatchAndStableRelevanceCursor(t *testing.T) {
	runtime := openContentRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewContentRepository(runtime)
	firstSource := createContentSource(t, runtime, "search-first")
	secondSource := createContentSource(t, runtime, "search-second")
	base := time.Date(2026, time.August, 1, 9, 0, 0, 0, time.UTC)
	inputs := []struct {
		sourceID  int64
		external  string
		published time.Time
	}{
		{firstSource, "发布-high", base.Add(3 * time.Hour)},
		{firstSource, "发布-middle", base.Add(2 * time.Hour)},
		{secondSource, "发布-other-source", base.Add(time.Hour)},
		{firstSource, "unrelated", base},
	}
	contents := make(map[string]ingestiondomain.Content, len(inputs))
	for _, fixture := range inputs {
		input := normalizedContent(fixture.sourceID, fixture.external, fixture.published)
		input.PublishedAt = fixture.published
		stored, _, err := repository.Upsert(context.Background(), input, activeDecision())
		if err != nil {
			t.Fatalf("Upsert(%s): %v", fixture.external, err)
		}
		contents[fixture.external] = stored
	}
	monitorID, configID := createContentSearchMonitor(t, runtime)
	insertContentSearchMatch(t, runtime, monitorID, configID, contents["发布-high"].ID, 10, ingestiondomain.MatchDecisionRejected, base)
	insertContentSearchMatch(t, runtime, monitorID, configID, contents["发布-high"].ID, 93, ingestiondomain.MatchDecisionAccepted, base.Add(time.Minute))
	insertContentSearchMatch(t, runtime, monitorID, configID, contents["发布-middle"].ID, 71, ingestiondomain.MatchDecisionReview, base)
	insertContentSearchMatch(t, runtime, monitorID, configID, contents["发布-other-source"].ID, 82, ingestiondomain.MatchDecisionAccepted, base)
	assertContentSearchIndexes(t, runtime, monitorID)

	from, to := base.Add(time.Hour), base.Add(4*time.Hour)
	filtered, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{
		Limit: 10, Keyword: "发布", SourceConnectionID: &firstSource, PublishedFrom: &from, PublishedTo: &to, Sort: ingestiondomain.ContentSortLatest,
	})
	if err != nil || len(filtered.Items) != 2 || filtered.Items[0].ExternalID != "发布-high" || filtered.Items[1].ExternalID != "发布-middle" {
		t.Fatalf("filtered page/error = %#v/%v", filtered, err)
	}

	accepted := ingestiondomain.MatchDecisionAccepted
	first, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{
		Limit: 1, Keyword: "发布", MonitorID: &monitorID, Sort: ingestiondomain.ContentSortRelevance,
	})
	if err != nil || len(first.Items) != 1 || first.Items[0].ExternalID != "发布-high" || first.Items[0].RelevanceScore == nil || *first.Items[0].RelevanceScore != 93 || first.Items[0].MatchDecision == nil || *first.Items[0].MatchDecision != accepted || first.NextCursor == "" {
		t.Fatalf("first relevance page/error = %#v/%v", first, err)
	}
	second, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{
		Limit: 1, Keyword: "发布", MonitorID: &monitorID, Sort: ingestiondomain.ContentSortRelevance, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 1 || second.Items[0].ExternalID != "发布-other-source" || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second relevance page/error = %#v/%v", second, err)
	}
	if _, err := repository.ListActive(context.Background(), ingestiondomain.ContentListQuery{
		Limit: 1, Keyword: "发布", MonitorID: &monitorID, Decision: &accepted, Sort: ingestiondomain.ContentSortRelevance, Cursor: first.NextCursor,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("changed shape cursor error = %v, want invalid input", err)
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
		"https://objects.example.test/evidence?x=1;X-Amz-Signature=opaque",
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

func createContentSearchMonitor(t *testing.T, runtime *database.Runtime) (int64, int64) {
	t.Helper()
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name, status) VALUES ($1, 'draft') RETURNING id`, fmt.Sprintf("content-search-%d", time.Now().UnixNano())).Scan(&monitorID); err != nil {
		t.Fatalf("create search monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create search monitor config: %v", err)
	}
	return monitorID, configID
}

func insertContentSearchMatch(t *testing.T, runtime *database.Runtime, monitorID, configID, contentID int64, score float64, decision ingestiondomain.MatchDecision, createdAt time.Time) {
	t.Helper()
	inputHash := fmt.Sprintf("%064x", contentID+int64(score)+createdAt.UnixNano())
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_matches (
  monitor_id, monitor_config_version_id, content_id, rule_score, final_score,
  decision, algorithm_version, input_hash, scoring_version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $4, $5, 'search-v1', $6, 'search-v1', $7, $7)`,
		monitorID, configID, contentID, score, string(decision), inputHash[len(inputHash)-64:], createdAt); err != nil {
		t.Fatalf("insert content search match: %v", err)
	}
}

func assertContentSearchIndexes(t *testing.T, runtime *database.Runtime, monitorID int64) {
	t.Helper()
	var plan strings.Builder
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		for _, statement := range []string{`ANALYZE contents`, `ANALYZE monitor_matches`, `SET LOCAL enable_seqscan = off`} {
			if _, err := transaction.SQL.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		rows, err := transaction.SQL.QueryContext(ctx, `
EXPLAIN (COSTS OFF)
SELECT c.id, latest_match.final_score
FROM contents AS c
JOIN LATERAL (
  SELECT match.final_score
  FROM monitor_matches AS match
  WHERE match.monitor_id=$1 AND match.content_id=c.id
  ORDER BY match.created_at DESC,match.id DESC LIMIT 1
) latest_match ON true
WHERE c.content_status='active' AND c.deleted_at IS NULL
  AND lower(c.title || ' ' || c.excerpt) LIKE '%发布%'
ORDER BY latest_match.final_score DESC,c.id DESC LIMIT 100`, monitorID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			plan.WriteString(line)
			plan.WriteByte('\n')
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"contents_search_active_trgm_idx", "monitor_matches_monitor_content_latest_idx"} {
		if !strings.Contains(plan.String(), index) {
			t.Fatalf("content search plan missing %s:\n%s", index, plan.String())
		}
	}
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
