package postgres_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
)

func TestSourceDocumentGenerationPublishesExactVaultArtifactsAndLexicalProjectionIdempotently(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "source-generation", 74)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t, runtime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)

	plaintext := "authorized normalized document body"
	markdown := "# Authorized\n\nnormalized document body"
	payload := []byte(`<rssItem><guid>source-generation</guid><encoded>authorized normalized document body</encoded></rssItem>`)
	evidence := ingestionapplication.SelectedSourceEvidenceDTO{
		EvidenceReferenceID: 701, SourceObservationID: fixture.observationID, EvidenceSnapshotID: 702,
		SourceConnectionID: fixture.sourceID, ExternalWorkID: "derived-artifact-source-generation",
		UpstreamIdentity: strings.Repeat("a", 64), SourceCode: "rss", ContentType: "article", Title: "Authorized",
		Language: "en", SourceRecordURL: "https://feed.example.test/source-generation",
		CanonicalURL: "https://publisher.example.test/articles/derived-artifact-source-generation",
		BodyOrigin:   ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
		CapturedAt: fixture.persisted.DocumentVersion.CapturedAt, SelectedPayload: payload,
		SelectedPayloadSHA256: fmt.Sprintf("%x", sha256.Sum256(payload)), PayloadMIMEType: "application/rss+xml",
		SelectorVersion: "rss2-go-xml-v1",
	}
	extraction := ingestionapplication.ExtractSelectedSourceBodyResult{
		BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
		Plaintext: plaintext, Markdown: markdown, Language: "en", ExtractorVersion: "rss-entry-v2",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("f", 64),
		PlaintextTransformerProfileSHA256: strings.Repeat("1", 64),
		MarkdownTransformerProfileSHA256:  strings.Repeat("2", 64),
		PlaintextSHA256:                   fixture.persisted.DocumentVersion.ContentSHA256,
	}
	vaultRoot := t.TempDir()
	artifactProjection := newDerivedArtifactSaga(
		t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions,
	)
	recallWriter, err := ingestionpostgres.NewDocumentRecallProjectionWriter(runtime)
	if err != nil {
		t.Fatalf("NewDocumentRecallProjectionWriter() error = %v", err)
	}
	recallProjection, err := ingestionapplication.NewDocumentRecallProjectionService(recallWriter)
	if err != nil {
		t.Fatalf("NewDocumentRecallProjectionService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	decisionAt := now
	generator, err := ingestionapplication.NewSourceDocumentGenerationService(ingestionapplication.SourceDocumentGenerationDependencies{
		Evidence: &sourceGenerationEvidenceReader{evidence: evidence}, Extractor: &sourceGenerationBodyExtractor{result: extraction},
		DocumentVersions: fixture.documentVersions,
		Authorizations:   ingestionpostgres.NewDocumentProjectionAuthorizationReader(runtime),
		Projections:      artifactProjection, SearchProjections: recallProjection, Now: func() time.Time { return decisionAt },
	})
	if err != nil {
		t.Fatalf("NewSourceDocumentGenerationService() error = %v", err)
	}

	first, err := generator.Generate(context.Background(), ingestionapplication.GenerateSourceDocumentCommand{EvidenceReferenceID: evidence.EvidenceReferenceID})
	if err != nil {
		t.Fatalf("Generate(first) error = %v", err)
	}
	if first.PlaintextAvailability != ingestionapplication.SourceDocumentAvailable ||
		first.SearchAvailability != ingestionapplication.SourceDocumentAvailable ||
		first.MarkdownAvailability != ingestionapplication.SourceDocumentAvailable ||
		first.PlaintextArtifact == nil || first.MarkdownArtifact == nil || first.SearchProjection == nil ||
		first.LastVerifiedDocumentLifecycleState != ingestionapplication.DocumentReadable ||
		first.PlaintextArtifact.StoreDerivedRightsDecisionID != storeDecisionID ||
		first.PlaintextArtifact.RetainRightsDecisionID != retainDecisionID ||
		first.MarkdownArtifact.StoreDerivedRightsDecisionID != storeDecisionID ||
		first.MarkdownArtifact.RetainRightsDecisionID != retainDecisionID {
		t.Fatalf("Generate(first) = %#v, display decision %d", first, displayDecisionID)
	}

	decisionAt = now.Add(time.Minute)
	second, err := generator.Generate(context.Background(), ingestionapplication.GenerateSourceDocumentCommand{EvidenceReferenceID: evidence.EvidenceReferenceID})
	if err != nil {
		t.Fatalf("Generate(retry) error = %v", err)
	}
	if second.PlaintextArtifact == nil || second.MarkdownArtifact == nil || second.SearchProjection == nil ||
		second.PlaintextArtifact.ID != first.PlaintextArtifact.ID || second.MarkdownArtifact.ID != first.MarkdownArtifact.ID ||
		second.SearchProjection.ProjectionID != first.SearchProjection.ProjectionID || second.SearchProjection.Created {
		t.Fatalf("Generate(retry) = %#v, want exact idempotent identities", second)
	}

	assertSourceGenerationVaultFile(t, vaultRoot, fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID,
		"plaintext", extraction.PlaintextTransformerProfileSHA256, ".txt", plaintext)
	assertSourceGenerationVaultFile(t, vaultRoot, fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID,
		"markdown", extraction.MarkdownTransformerProfileSHA256, ".md", markdown)
	var artifactCount, searchProjectionCount int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM derived_artifacts
WHERE document_version_id=$1 AND lifecycle_state='derived_available' AND active`, fixture.persisted.DocumentVersion.ID).Scan(&artifactCount); err != nil {
		t.Fatalf("count active derived artifacts: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM document_version_search_indexes
WHERE document_version_id=$1 AND normalization_profile_version=$2 AND lifecycle_state='active'`,
		fixture.persisted.DocumentVersion.ID, ingestionapplication.CanonicalDocumentSearchNormalizationProfileVersion,
	).Scan(&searchProjectionCount); err != nil {
		t.Fatalf("count active search projections: %v", err)
	}
	if artifactCount != 2 || searchProjectionCount != 1 {
		t.Fatalf("active artifact/search counts = %d/%d, want 2/1", artifactCount, searchProjectionCount)
	}
}

type sourceGenerationEvidenceReader struct {
	evidence ingestionapplication.SelectedSourceEvidenceDTO
}

func (reader *sourceGenerationEvidenceReader) ReadSelectedSourceEvidence(_ context.Context, query ingestionapplication.SourceEvidenceQuery) (ingestionapplication.SelectedSourceEvidenceDTO, error) {
	if query.EvidenceReferenceID != reader.evidence.EvidenceReferenceID {
		return ingestionapplication.SelectedSourceEvidenceDTO{}, fmt.Errorf("unexpected evidence reference")
	}
	result := reader.evidence
	result.SelectedPayload = append([]byte(nil), reader.evidence.SelectedPayload...)
	return result, nil
}

type sourceGenerationBodyExtractor struct {
	result ingestionapplication.ExtractSelectedSourceBodyResult
}

func (extractor *sourceGenerationBodyExtractor) Extract(context.Context, ingestionapplication.ExtractSelectedSourceBodyCommand) (ingestionapplication.ExtractSelectedSourceBodyResult, error) {
	return extractor.result, nil
}

func assertSourceGenerationVaultFile(t *testing.T, root string, documentID, documentVersionID int64, artifactType, profile, extension, want string) {
	t.Helper()
	path := filepath.Join(root, "documents", fmt.Sprint(documentID), fmt.Sprint(documentVersionID), artifactType, profile+extension)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s Vault artifact: %v", artifactType, err)
	}
	if string(content) != want {
		t.Fatalf("%s Vault artifact changed", artifactType)
	}
}
