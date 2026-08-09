package postgres_test

import (
	"context"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestRecallPreviewDocumentReaderPreservesOrderAndHidesTitleAfterRightsWithdrawal(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	first := createDerivedArtifactDocument(t, runtime, "preview-title-first", 71)
	second := createDerivedArtifactDocument(t, runtime, "preview-title-second", 72)
	makePreviewDocumentReadable(t, runtime, first, 11)
	makePreviewDocumentReadable(t, runtime, second, 21)
	reader, err := ingestionpostgres.NewHybridDocumentRecallReader(runtime)
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{second.persisted.DocumentVersion.ID, first.persisted.DocumentVersion.ID}

	visible, err := reader.ReadRecallPreviewDocuments(context.Background(), ingestionapplication.RecallPreviewDocumentQuery{DocumentVersionIDs: ids})
	if err != nil {
		t.Fatalf("ReadRecallPreviewDocuments(): %v", err)
	}
	if len(visible.Documents) != 2 || visible.Documents[0].DocumentVersionID != ids[0] || visible.Documents[1].DocumentVersionID != ids[1] ||
		!visible.Documents[0].TitleAvailable || visible.Documents[0].Title == "" || !visible.Documents[1].TitleAvailable {
		t.Fatalf("visible ordered documents = %#v", visible.Documents)
	}

	denyPolicy := createDocumentObservationRightsPolicy(t, runtime, second.sourceID, second.observationID, 22, time.Now().UTC().Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, second.persisted.DocumentVersion.ID,
		second.persisted.DocumentVersion.ContentSHA256, "display_private", "deny", nil, nil, second.persisted.DocumentVersion.ID)
	hidden, err := reader.ReadRecallPreviewDocuments(context.Background(), ingestionapplication.RecallPreviewDocumentQuery{DocumentVersionIDs: ids})
	if err != nil {
		t.Fatalf("ReadRecallPreviewDocuments(after withdrawal): %v", err)
	}
	if hidden.Documents[0].TitleAvailable || hidden.Documents[0].Title != "" || !hidden.Documents[1].TitleAvailable {
		t.Fatalf("rights-safe title projection = %#v", hidden.Documents)
	}
}

func makePreviewDocumentReadable(t *testing.T, runtime *database.Runtime, fixture derivedArtifactDocumentFixture, policyRevision int64) {
	t.Helper()
	transitionDocumentVersion(t, fixture.documentVersions, fixture.persisted.DocumentVersion.ID, 1, ingestionapplication.DocumentDerivedPending)
	createAvailableDocumentArtifact(t, runtime, fixture.sourceID, fixture.persisted.Document.ID,
		fixture.persisted.DocumentVersion.ID, fixture.persisted.DocumentVersion.ContentSHA256)
	derived := transitionDocumentVersion(t, fixture.documentVersions, fixture.persisted.DocumentVersion.ID, 2, ingestionapplication.DocumentDerivedAvailable)
	displayDecisionID := createDocumentDisplayDecision(t, runtime, fixture.sourceID,
		fixture.persisted.DocumentVersion.ID, fixture.persisted.DocumentVersion.ContentSHA256, policyRevision, nil,
		fixture.persisted.DocumentVersion.ID)
	transitionDocumentVersionWithDisplay(t, fixture.documentVersions, fixture.persisted.DocumentVersion.ID, derived.Version, displayDecisionID)
}
