package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
)

func TestContentRepositorySearchUsesCurrentPostgresLexicalProjection(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "global-lexical-search", 81)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t, runtime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)
	plaintext := []byte("authorized normalized document body")
	projection := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, t.TempDir()), fixture.documentVersions)
	projected, err := projection.Project(context.Background(), ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, ExpectedDocumentVersion: fixture.persisted.DocumentVersion.Version,
		ArtifactType: ingestionapplication.DocumentProjectionPlaintext, TransformerProfileSHA256: strings.Repeat("8", 64),
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		ProjectionBytes: plaintext,
	})
	if err != nil {
		t.Fatal(err)
	}
	markdown := []byte("# Archived\n\nauthorized normalized document body\n")
	readableCommand := derivedArtifactProjectCommand(
		fixture, strings.Repeat("7", 64), markdown, storeDecisionID, retainDecisionID, &displayDecisionID,
	)
	readableCommand.ExpectedDocumentVersion = projected.DocumentVersion.Version
	if _, err := projection.Project(context.Background(), readableCommand); err != nil {
		t.Fatal(err)
	}
	writer, err := ingestionpostgres.NewDocumentRecallProjectionWriter(runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := ingestionapplication.NewDocumentRecallProjectionService(writer)
	if err != nil {
		t.Fatal(err)
	}
	indexedAt := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := service.PersistSearchProjection(context.Background(), ingestionapplication.PersistDocumentSearchProjectionCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, DerivedArtifactID: projected.Artifact.ID,
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		NormalizationProfileVersion: "global-lexical-v1", NormalizedTextSHA256: fixture.persisted.DocumentVersion.ContentSHA256,
		Plaintext: string(plaintext), EntityKeys: []string{"OpenAI", "芯片实验室"}, ActionKeys: []string{"release"},
		LocationKeys: []string{"Shanghai"}, RegionKeys: []string{"CN"}, IndexedAt: indexedAt,
	}); err != nil {
		t.Fatal(err)
	}
	var externalID string
	if err := runtime.SQL.QueryRow(`SELECT external_work_id FROM documents WHERE id=$1`, fixture.persisted.Document.ID).Scan(&externalID); err != nil {
		t.Fatal(err)
	}
	publishedAt := indexedAt.Add(-time.Hour)
	var contentID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (source_connection_id,external_id,content_type,title,excerpt,canonical_url,language,published_at,fetched_at,dedupe_key)
VALUES ($1,$2,'article',$3,$4,'https://example.test/lexical','zh',$5,$5,$6) RETURNING id`,
		fixture.sourceID, externalID, `<img src=x onerror=sentinel> 芯片 AI release`, `上海芯片发布摘要 javascript:sentinel`, publishedAt, strings.Repeat("9", 64)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	repository := ingestionpostgres.NewContentRepository(runtime)

	queries := []searchdomain.Query{
		{Keyword: "芯片"},
		{Keyword: "AI"},
		{Keyword: "releas"},
		{Keyword: "authorized"},
		{Keyword: "authorized", Entity: "openai"},
		{Keyword: "release", SourceConnectionID: &fixture.sourceID, Status: "active", From: pointerTime(publishedAt.Add(-time.Minute)), To: pointerTime(publishedAt.Add(time.Minute))},
	}
	for _, query := range queries {
		query.Types = []searchdomain.ResourceType{searchdomain.ResourceContent}
		query.Limit = 10
		items, err := repository.Search(context.Background(), query.Normalized())
		if err != nil || len(items) != 1 || items[0].Type != searchdomain.ResourceContent || items[0].ID != contentID || items[0].SourceConnectionID != fixture.sourceID || items[0].Title == "" || items[0].OccurredAt.IsZero() || items[0].Score < 0 {
			t.Fatalf("Search(%#v) = %#v/%v", query, items, err)
		}
	}

	otherSource := fixture.sourceID + 999999
	items, err := repository.Search(context.Background(), searchdomain.Query{Keyword: "release", SourceConnectionID: &otherSource, Types: []searchdomain.ResourceType{searchdomain.ResourceContent}, Limit: 10}.Normalized())
	if err != nil || len(items) != 0 {
		t.Fatalf("Search(other source) = %#v/%v", items, err)
	}
	visibilityQuery := searchdomain.Query{Keyword: "release", Types: []searchdomain.ResourceType{searchdomain.ResourceContent}, Limit: 10}.Normalized()
	visibleItems, err := repository.Search(context.Background(), visibilityQuery)
	if err != nil || len(visibleItems) != 1 {
		t.Fatalf("Search(visibility) = %#v/%v", visibleItems, err)
	}
	if visible, err := repository.CanDisplay(context.Background(), visibilityQuery, visibleItems[0]); err != nil || !visible {
		t.Fatalf("CanDisplay(active) = %v/%v", visible, err)
	}
	revocationPolicy := createDocumentRightsPolicy(t, runtime, fixture.sourceID, 3, time.Now().UTC().Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(t, runtime, revocationPolicy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "store_derived", "deny", nil, nil, fixture.persisted.DocumentVersion.ID)
	if visible, err := repository.CanDisplay(context.Background(), visibilityQuery, visibleItems[0]); err != nil || visible {
		t.Fatalf("CanDisplay(rights revoked) = %v/%v", visible, err)
	}
}

func pointerTime(value time.Time) *time.Time { return &value }
