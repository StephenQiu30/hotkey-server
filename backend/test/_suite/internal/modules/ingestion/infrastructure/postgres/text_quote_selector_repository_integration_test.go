package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
)

func TestTextQuoteSelectorRepositoryPersistsExactAuthorizedUTF8SelectorIdempotently(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "text-quote", 88)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(t, runtime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID)
	quotePolicy := createDocumentRightsPolicy(t, runtime, fixture.sourceID, 3, time.Now().UTC().Add(-time.Hour))
	quoteDecisionID := insertDocumentEndpointRightsDecision(t, runtime, quotePolicy, "quote", "allow", nil)
	if quoteDecisionID <= 0 {
		t.Fatal("quote decision was not created")
	}

	vaultService, err := knowledgeapplication.NewProjectionService(knowledgevault.NewWriter(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	saga := newDerivedArtifactSaga(t, runtime, vaultService, fixture.documentVersions)
	plaintext := "authorized normalized document body"
	plaintextResult, err := saga.Project(context.Background(), ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, ExpectedDocumentVersion: fixture.persisted.DocumentVersion.Version,
		ArtifactType: ingestionapplication.DocumentProjectionPlaintext, TransformerProfileSHA256: strings.Repeat("c", 64),
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		ProjectionBytes: []byte(plaintext),
	})
	if err != nil {
		t.Fatalf("Project(plaintext): %v", err)
	}
	markdownCommand := derivedArtifactProjectCommand(fixture, strings.Repeat("d", 64), []byte("authorized normalized document body"),
		storeDecisionID, retainDecisionID, &displayDecisionID)
	markdownCommand.ExpectedDocumentVersion = plaintextResult.DocumentVersion.Version
	markdownResult, err := saga.Project(context.Background(), markdownCommand)
	if err != nil || markdownResult.DocumentVersion.LifecycleState != ingestionapplication.DocumentReadable {
		t.Fatalf("Project(markdown): %#v / %v", markdownResult, err)
	}

	repository := ingestionpostgres.NewTextQuoteSelectorRepository(runtime)
	service, err := ingestionapplication.NewTextQuoteSelectorService(ingestionapplication.TextQuoteSelectorDependencies{
		Repository: repository, Projections: vaultService,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := int64(len("authorized "))
	end := start + int64(len("normalized"))
	prefix, suffix := quoteContext(plaintext, start, end)
	now := time.Now().UTC().Truncate(time.Microsecond)
	command := ingestionapplication.CreateTextQuoteSelectorCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, ExactQuote: "normalized", Prefix: prefix, Suffix: suffix,
		UTF8ByteStart: start, UTF8ByteEnd: end, PlaintextSHA256: fixture.persisted.DocumentVersion.ContentSHA256,
		NormalizationVersion: ingestionapplication.CanonicalDocumentTextNormalizationVersion, DecisionAt: now,
	}
	first, err := service.Create(context.Background(), command)
	if err != nil {
		t.Fatalf("Create(first): %v", err)
	}
	second, err := service.Create(context.Background(), command)
	if err != nil || second.Selector.ID != first.Selector.ID || second.Selector.QuoteRightsDecisionID != quoteDecisionID ||
		second.Selector.MarkdownAnchor == nil || *second.Selector.MarkdownAnchor != markdownCommand.AnchorMap.Blocks[0].MarkdownAnchor {
		t.Fatalf("Create(retry): first=%#v second=%#v error=%v", first, second, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE document_text_quote_selectors SET exact_quote='changed' WHERE id=$1`, first.Selector.ID); err == nil {
		t.Fatal("append-only quote selector accepted UPDATE")
	}

	denyPolicy := createDocumentObservationRightsPolicy(t, runtime, fixture.sourceID, fixture.observationID, 4, now.Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "quote", "deny", nil, nil, fixture.persisted.DocumentVersion.ID)
	if _, err := service.Create(context.Background(), command); err == nil {
		t.Fatal("current higher-priority quote deny did not fail closed")
	}
	var selectorCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_text_quote_selectors WHERE document_version_id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&selectorCount); err != nil {
		t.Fatal(err)
	}
	if selectorCount != 1 {
		t.Fatalf("selector count after deny = %d", selectorCount)
	}
}

func quoteContext(plaintext string, start, end int64) (string, string) {
	return plaintext[:start], plaintext[end:]
}
