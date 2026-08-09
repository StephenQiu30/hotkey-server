package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestSourceDocumentGenerationPublishesPlaintextIndexesExactReceiptThenPublishesMarkdown(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,index,project_markdown"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if fixture.extractor.command.Evidence.EvidenceReferenceID != 71 || string(fixture.extractor.command.Evidence.SelectedPayload) != string(fixture.reader.evidence.SelectedPayload) {
		t.Fatalf("extract command = %#v", fixture.extractor.command)
	}
	persist := fixture.persister.command
	if persist.Observation.ID != 41 || persist.Observation.SourceConnectionID != 7 || persist.Observation.ExternalWorkID != "article-41" ||
		persist.Observation.Body != "Café launch" || persist.Observation.BodyOrigin != BodyOriginFeedContent ||
		persist.Observation.Completeness != BodyCompletenessFull || persist.ExtractorVersion != "feed-body-extractor-v1" ||
		persist.ExtractorProfileVersion != "rss-atom-rdf-body-v1" || persist.ExtractorProfileSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("PersistDocumentObservation() command = %#v", persist)
	}
	if got := fixture.authorization.query; got.SourceConnectionID != 7 || got.DocumentVersionID != 29 ||
		got.ContentSHA256 != fixture.extraction.PlaintextSHA256 || !got.DecisionAt.Equal(fixture.now) {
		t.Fatalf("authorization query = %#v", got)
	}
	if len(fixture.projector.commands) != 2 {
		t.Fatalf("Project() command count = %d", len(fixture.projector.commands))
	}
	plaintextProject := fixture.projector.commands[0]
	if plaintextProject.DocumentVersionID != 29 || plaintextProject.ExpectedDocumentVersion != 1 || plaintextProject.ArtifactType != DocumentProjectionPlaintext ||
		plaintextProject.StoreDerivedRightsDecisionID != 81 || plaintextProject.RetainRightsDecisionID != 82 ||
		plaintextProject.DisplayPrivateRightsDecisionID != nil ||
		plaintextProject.TransformerProfileSHA256 != strings.Repeat("c", 64) || string(plaintextProject.ProjectionBytes) != "Café launch" {
		t.Fatalf("Project(plaintext) command = %#v", plaintextProject)
	}
	search := fixture.search.command
	if search.DocumentVersionID != 29 || search.DerivedArtifactID != 38 ||
		search.StoreDerivedRightsDecisionID != 81 || search.RetainRightsDecisionID != 82 ||
		search.NormalizationProfileVersion != CanonicalDocumentSearchNormalizationProfileVersion ||
		search.NormalizedTextSHA256 != fixture.extraction.PlaintextSHA256 || search.Plaintext != "Café launch" ||
		len(search.EntityKeys) != 0 || len(search.ActionKeys) != 0 || len(search.LocationKeys) != 0 || len(search.RegionKeys) != 0 ||
		!search.IndexedAt.Equal(fixture.now) {
		t.Fatalf("PersistSearchProjection() command = %#v", search)
	}
	markdownProject := fixture.projector.commands[1]
	if markdownProject.DocumentVersionID != 29 || markdownProject.ExpectedDocumentVersion != 3 || markdownProject.ArtifactType != DocumentProjectionMarkdown ||
		markdownProject.StoreDerivedRightsDecisionID != 81 || markdownProject.RetainRightsDecisionID != 82 ||
		markdownProject.DisplayPrivateRightsDecisionID == nil || *markdownProject.DisplayPrivateRightsDecisionID != 83 ||
		markdownProject.TransformerProfileSHA256 != strings.Repeat("b", 64) || string(markdownProject.ProjectionBytes) != "# Café launch" {
		t.Fatalf("Project(markdown) command = %#v", markdownProject)
	}
	if result.PlaintextAvailability != SourceDocumentAvailable || result.MarkdownAvailability != SourceDocumentAvailable ||
		result.SearchAvailability != SourceDocumentAvailable || result.DocumentID != 19 || result.DocumentVersionID != 29 ||
		result.LastVerifiedDocumentVersion != 4 || result.LastVerifiedDocumentLifecycleState != DocumentReadable ||
		result.PlaintextArtifact == nil || result.PlaintextArtifact.ID != 38 || result.MarkdownArtifact == nil || result.MarkdownArtifact.ID != 39 ||
		result.SearchProjection == nil || result.SearchProjection.ProjectionID != 49 {
		t.Fatalf("Generate() result = %#v", result)
	}
	resultType := reflect.TypeOf(result)
	for _, forbidden := range []string{"SelectedPayload", "ProjectionBytes", "Markdown", "Plaintext", "Body"} {
		if _, exposed := resultType.FieldByName(forbidden); exposed {
			t.Fatalf("GenerateSourceDocumentResult exposes %s", forbidden)
		}
	}
}

func TestSourceDocumentGenerationPersistsMetadataOnlyWithoutRightsOrArtifact(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessMetadataOnly)
	fixture.reader.evidence.BodyOrigin = BodyOriginFeedSummary
	fixture.extraction = ExtractSelectedSourceBodyResult{
		BodyOrigin: BodyOriginFeedSummary, Completeness: BodyCompletenessMetadataOnly,
		Language: "und", ExtractorVersion: "feed-body-extractor-v1", ExtractorProfileVersion: "rss-atom-rdf-body-v1",
		ExtractorProfileSHA256: strings.Repeat("a", 64), PlaintextTransformerProfileSHA256: strings.Repeat("c", 64),
		MarkdownTransformerProfileSHA256: strings.Repeat("b", 64), QualityWarnings: []string{"metadata_only"},
	}
	fixture.extractor.result = fixture.extraction
	fixture.persister.result.DocumentVersion.BodyOrigin = BodyOriginFeedSummary
	fixture.persister.result.DocumentVersion.Completeness = BodyCompletenessMetadataOnly
	fixture.persister.result.DocumentVersion.ContentSHA256 = fmt.Sprintf("%x", sha256.Sum256(nil))

	result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if fixture.persister.command.Observation.Body != "" || fixture.authorization.calls != 0 || fixture.projector.calls != 0 || fixture.search.calls != 0 {
		t.Fatalf("metadata-only side effects = persist %#v, auth %d, project %d, search %d", fixture.persister.command, fixture.authorization.calls, fixture.projector.calls, fixture.search.calls)
	}
	if result.PlaintextAvailability != SourceDocumentNotApplicable || result.MarkdownAvailability != SourceDocumentNotApplicable ||
		result.SearchAvailability != SourceDocumentNotApplicable || result.PlaintextArtifact != nil || result.MarkdownArtifact != nil ||
		result.SearchProjection != nil || result.ContentSHA256 != "" || result.DocumentVersionID != 29 ||
		result.LastVerifiedDocumentLifecycleState != DocumentPolicyPending {
		t.Fatalf("metadata-only result = %#v", result)
	}
}

func TestSourceDocumentGenerationReturnsHonestPartialResultWhenSearchProjectionFails(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.search.err = sharedrepository.ErrUnavailable
	result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want search unavailable", err)
	}
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,index"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if result.PlaintextAvailability != SourceDocumentAvailable || result.PlaintextArtifact == nil || result.PlaintextArtifact.ID != 38 ||
		result.SearchAvailability != SourceDocumentUnavailable || result.SearchProjection != nil ||
		result.MarkdownAvailability != SourceDocumentUnavailable || result.MarkdownArtifact != nil ||
		result.LastVerifiedDocumentLifecycleState != DocumentDerivedAvailable || fixture.projector.calls != 1 {
		t.Fatalf("partial search failure result = %#v", result)
	}
}

func TestSourceDocumentGenerationReturnsHonestPartialResultWhenMarkdownProjectionFails(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.projector.errors[DocumentProjectionMarkdown] = sharedrepository.ErrUnavailable
	result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want Markdown unavailable", err)
	}
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,index,project_markdown"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if result.PlaintextAvailability != SourceDocumentAvailable || result.SearchAvailability != SourceDocumentAvailable ||
		result.SearchProjection == nil || result.MarkdownAvailability != SourceDocumentUnavailable || result.MarkdownArtifact != nil ||
		result.LastVerifiedDocumentLifecycleState != DocumentDerivedAvailable {
		t.Fatalf("partial Markdown failure result = %#v", result)
	}
}

func TestSourceDocumentGenerationRejectsEveryMismatchedDownstreamReceiptBeforeNextEffect(t *testing.T) {
	t.Parallel()

	t.Run("plaintext rights receipt", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		result := fixture.projector.results[DocumentProjectionPlaintext]
		result.Artifact.RetainRightsDecisionID = 999
		fixture.projector.results[DocumentProjectionPlaintext] = result
		generated, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.search.calls != 0 || fixture.projector.calls != 1 ||
			generated.PlaintextAvailability != SourceDocumentUnavailable {
			t.Fatalf("plaintext receipt mismatch = %#v/%v, calls project=%d search=%d", generated, err, fixture.projector.calls, fixture.search.calls)
		}
	})

	t.Run("search identity receipt", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.search.result.NormalizedTextSHA256 = strings.Repeat("f", 64)
		generated, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.projector.calls != 1 ||
			generated.PlaintextAvailability != SourceDocumentAvailable || generated.SearchAvailability != SourceDocumentUnavailable ||
			generated.MarkdownAvailability != SourceDocumentUnavailable {
			t.Fatalf("search receipt mismatch = %#v/%v, project calls=%d", generated, err, fixture.projector.calls)
		}
	})

	t.Run("search plaintext artifact receipt", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.search.result.DerivedArtifactID = 999
		generated, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.projector.calls != 1 ||
			generated.PlaintextAvailability != SourceDocumentAvailable || generated.SearchAvailability != SourceDocumentUnavailable ||
			generated.MarkdownAvailability != SourceDocumentUnavailable {
			t.Fatalf("search artifact receipt mismatch = %#v/%v, project calls=%d", generated, err, fixture.projector.calls)
		}
	})

	t.Run("search rights receipt", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.search.result.RetainRightsDecisionID = 999
		generated, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.projector.calls != 1 ||
			generated.PlaintextAvailability != SourceDocumentAvailable || generated.SearchAvailability != SourceDocumentUnavailable ||
			generated.MarkdownAvailability != SourceDocumentUnavailable {
			t.Fatalf("search rights receipt mismatch = %#v/%v, project calls=%d", generated, err, fixture.projector.calls)
		}
	})

	t.Run("search retention receipt", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.search.result.RetentionUntil = fixture.now.Add(29 * 24 * time.Hour)
		generated, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.projector.calls != 1 ||
			generated.PlaintextAvailability != SourceDocumentAvailable || generated.SearchAvailability != SourceDocumentUnavailable ||
			generated.MarkdownAvailability != SourceDocumentUnavailable {
			t.Fatalf("search retention receipt mismatch = %#v/%v, project calls=%d", generated, err, fixture.projector.calls)
		}
	})

	t.Run("Markdown content receipt", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		result := fixture.projector.results[DocumentProjectionMarkdown]
		result.Artifact.SHA256 = strings.Repeat("f", 64)
		fixture.projector.results[DocumentProjectionMarkdown] = result
		generated, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.projector.calls != 2 ||
			generated.PlaintextAvailability != SourceDocumentAvailable || generated.SearchAvailability != SourceDocumentAvailable ||
			generated.MarkdownAvailability != SourceDocumentUnavailable {
			t.Fatalf("Markdown receipt mismatch = %#v/%v, project calls=%d", generated, err, fixture.projector.calls)
		}
	})
}

func TestSourceDocumentGenerationRejectsTamperedSelectedBytesBeforeExtraction(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.reader.evidence.SelectedPayload = []byte("tampered")
	_, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Generate() error = %v, want evidence conflict", err)
	}
	if fixture.extractor.calls != 0 || fixture.persister.calls != 0 || fixture.authorization.calls != 0 || fixture.projector.calls != 0 || fixture.search.calls != 0 {
		t.Fatal("tampered evidence reached downstream services")
	}
}

func TestSourceDocumentGenerationFailsClosedOnMismatchedDerivedAuthorization(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.authorization.result.ContentSHA256 = strings.Repeat("f", 64)
	_, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Generate() error = %v, want authorization conflict", err)
	}
	if fixture.authorization.calls != 1 || fixture.projector.calls != 0 || fixture.search.calls != 0 {
		t.Fatalf("authorization/project calls = %d/%d", fixture.authorization.calls, fixture.projector.calls)
	}
}

func TestSourceDocumentGenerationLeavesTOCTOURightsFailureToProjectionGate(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.projector.errors[DocumentProjectionPlaintext] = sharedrepository.ErrConflict
	_, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Generate() error = %v, want projection rights conflict", err)
	}
	if fixture.authorization.calls != 1 || fixture.projector.calls != 1 || fixture.search.calls != 0 {
		t.Fatalf("authorization/project calls = %d/%d", fixture.authorization.calls, fixture.projector.calls)
	}
}

func TestSourceDocumentGenerationRejectsExtractorPromotionAndPersistenceMismatch(t *testing.T) {
	t.Parallel()

	t.Run("extractor promotes declared summary", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessSummary)
		fixture.reader.evidence.BodyOrigin = BodyOriginFeedSummary
		fixture.extraction.BodyOrigin = BodyOriginFeedContent
		fixture.extraction.Completeness = BodyCompletenessFull
		_, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.persister.calls != 0 {
			t.Fatalf("promoted extractor output error/calls = %v/%d", err, fixture.persister.calls)
		}
	})

	t.Run("persistence returns another content digest", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.persister.result.DocumentVersion.ContentSHA256 = strings.Repeat("f", 64)
		_, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.authorization.calls != 0 {
			t.Fatalf("persistence mismatch error/auth calls = %v/%d", err, fixture.authorization.calls)
		}
	})
}

type sourceDocumentGenerationFixture struct {
	service       *SourceDocumentGenerationService
	reader        *selectedSourceEvidenceReaderFake
	extractor     *selectedSourceBodyExtractorFake
	persister     *documentObservationPersisterFake
	authorization *documentProjectionAuthorizationReaderFake
	projector     *documentArtifactProjectorFake
	search        *documentSearchProjectionPersisterFake
	extraction    ExtractSelectedSourceBodyResult
	calls         []string
	now           time.Time
}

func newSourceDocumentGenerationFixture(t *testing.T, completeness string) *sourceDocumentGenerationFixture {
	t.Helper()
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	payload := []byte(`<rssItem><guid>article-41</guid><encoded>&lt;p&gt;Café launch&lt;/p&gt;</encoded></rssItem>`)
	payloadSHA := fmt.Sprintf("%x", sha256.Sum256(payload))
	fixture := &sourceDocumentGenerationFixture{now: now}
	fixture.reader = &selectedSourceEvidenceReaderFake{callsLog: &fixture.calls, evidence: SelectedSourceEvidenceDTO{
		EvidenceReferenceID: 71, SourceObservationID: 41, EvidenceSnapshotID: 51, SourceConnectionID: 7,
		ExternalWorkID: "article-41", UpstreamIdentity: strings.Repeat("c", 64), SourceCode: "rss", ContentType: "article", Title: "Launch", Language: "en",
		SourceRecordURL: "https://feed.example.test/rss.xml", CanonicalURL: "https://news.example.test/launch",
		BodyOrigin: BodyOriginFeedContent, Completeness: completeness, CapturedAt: now.Add(-time.Minute),
		SelectedPayload: payload, SelectedPayloadSHA256: payloadSHA, PayloadMIMEType: "application/rss+xml", SelectorVersion: "rss2-go-xml-v1",
	}}
	fixture.extraction = ExtractSelectedSourceBodyResult{
		BodyOrigin: BodyOriginFeedContent, Completeness: completeness,
		Plaintext: "Café launch", Markdown: "# Café launch", Language: "en",
		ExtractorVersion: "feed-body-extractor-v1", ExtractorProfileVersion: "rss-atom-rdf-body-v1",
		ExtractorProfileSHA256: strings.Repeat("a", 64), PlaintextTransformerProfileSHA256: strings.Repeat("c", 64),
		MarkdownTransformerProfileSHA256: strings.Repeat("b", 64),
		PlaintextSHA256:                  fmt.Sprintf("%x", sha256.Sum256([]byte("Café launch"))),
	}
	fixture.extractor = &selectedSourceBodyExtractorFake{callsLog: &fixture.calls, result: fixture.extraction}
	capturedAt := now.Add(-time.Minute)
	fixture.persister = &documentObservationPersisterFake{callsLog: &fixture.calls, result: PersistDocumentVersionResult{
		Document: DocumentDTO{
			ID: 19, Version: 1, SourceConnectionID: 7, DocumentKey: strings.Repeat("d", 64),
			State: DocumentStateActive, CreatedAt: capturedAt, UpdatedAt: capturedAt,
		},
		DocumentCreated: true,
		DocumentVersion: DocumentVersionDTO{
			ID: 29, Version: 1, DocumentID: 19, SourceObservationID: 41, RevisionNo: 1,
			VersionKey: strings.Repeat("e", 64), BodyOrigin: BodyOriginFeedContent,
			Completeness: completeness, WordCount: 2, Language: "en",
			ContentSHA256: fixture.extraction.PlaintextSHA256, LifecycleState: DocumentPolicyPending,
			ExtractorVersion: "feed-body-extractor-v1", ExtractorProfileVersion: "rss-atom-rdf-body-v1",
			ExtractorProfileSHA256: strings.Repeat("a", 64), CapturedAt: capturedAt,
			CreatedAt: capturedAt, UpdatedAt: capturedAt,
		},
		DocumentVersionCreated: true,
	}}
	displayID := int64(83)
	fixture.authorization = &documentProjectionAuthorizationReaderFake{callsLog: &fixture.calls, result: DocumentProjectionAuthorizationDTO{
		SourceConnectionID: 7, DocumentVersionID: 29, ContentSHA256: fixture.extraction.PlaintextSHA256,
		DecisionAt: now, StoreDerivedRightsDecisionID: 81, RetainRightsDecisionID: 82, DisplayPrivateRightsDecisionID: &displayID,
	}}
	availableAt := now
	plaintextVersion := fixture.persister.result.DocumentVersion
	plaintextVersion.Version = 3
	plaintextVersion.LifecycleState = DocumentDerivedAvailable
	markdownVersion := plaintextVersion
	markdownVersion.Version = 4
	markdownVersion.LifecycleState = DocumentReadable
	fixture.projector = &documentArtifactProjectorFake{callsLog: &fixture.calls, errors: map[string]error{}, results: map[string]ProjectDocumentResult{
		DocumentProjectionPlaintext: {
			Artifact: DerivedArtifactDTO{
				ID: 38, SourceConnectionID: 7, DocumentVersionID: 29,
				StoreDerivedRightsDecisionID: 81, RetainRightsDecisionID: 82,
				ArtifactType: DocumentProjectionPlaintext, Active: true,
				TransformerProfileSHA256: strings.Repeat("c", 64), SHA256: fixture.extraction.PlaintextSHA256,
				MIMEType: "text/plain; charset=utf-8", SizeBytes: int64(len("Café launch")), LifecycleState: DerivedArtifactAvailable,
				AvailableAt: &availableAt, RetentionUntil: now.Add(30 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now,
			},
			DocumentVersion: plaintextVersion,
		},
		DocumentProjectionMarkdown: {
			Artifact: DerivedArtifactDTO{
				ID: 39, SourceConnectionID: 7, DocumentVersionID: 29,
				StoreDerivedRightsDecisionID: 81, RetainRightsDecisionID: 82,
				ArtifactType: DocumentProjectionMarkdown, Active: true,
				TransformerProfileSHA256: strings.Repeat("b", 64), SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("# Café launch"))),
				MIMEType: "text/markdown; charset=utf-8", SizeBytes: int64(len("# Café launch")), LifecycleState: DerivedArtifactAvailable,
				AvailableAt: &availableAt, RetentionUntil: now.Add(30 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now,
			},
			DocumentVersion: markdownVersion,
		},
	}}
	fixture.search = &documentSearchProjectionPersisterFake{callsLog: &fixture.calls, result: DocumentSearchProjectionResult{
		ProjectionID: 49, DocumentVersionID: 29, SourceConnectionID: 7, DerivedArtifactID: 38,
		StoreDerivedRightsDecisionID: 81, RetainRightsDecisionID: 82,
		NormalizationProfileVersion: CanonicalDocumentSearchNormalizationProfileVersion,
		NormalizedTextSHA256:        fixture.extraction.PlaintextSHA256,
		RetentionUntil:              now.Add(30 * 24 * time.Hour), IndexedAt: now,
		LifecycleState: RecallAssetLifecycleActive, Created: true,
	}}
	service, err := NewSourceDocumentGenerationService(SourceDocumentGenerationDependencies{
		Evidence: fixture.reader, Extractor: fixture.extractor, DocumentVersions: fixture.persister,
		Authorizations: fixture.authorization, Projections: fixture.projector, SearchProjections: fixture.search,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSourceDocumentGenerationService() error = %v", err)
	}
	fixture.service = service
	return fixture
}

type selectedSourceEvidenceReaderFake struct {
	callsLog *[]string
	evidence SelectedSourceEvidenceDTO
	err      error
}

func (reader *selectedSourceEvidenceReaderFake) ReadSelectedSourceEvidence(_ context.Context, query SourceEvidenceQuery) (SelectedSourceEvidenceDTO, error) {
	*reader.callsLog = append(*reader.callsLog, "read")
	if query.EvidenceReferenceID != reader.evidence.EvidenceReferenceID {
		return SelectedSourceEvidenceDTO{}, sharedrepository.ErrConflict
	}
	return reader.evidence, reader.err
}

type selectedSourceBodyExtractorFake struct {
	callsLog *[]string
	command  ExtractSelectedSourceBodyCommand
	result   ExtractSelectedSourceBodyResult
	err      error
	calls    int
}

func (extractor *selectedSourceBodyExtractorFake) Extract(_ context.Context, command ExtractSelectedSourceBodyCommand) (ExtractSelectedSourceBodyResult, error) {
	*extractor.callsLog = append(*extractor.callsLog, "extract")
	extractor.calls++
	extractor.command = command
	return extractor.result, extractor.err
}

type documentObservationPersisterFake struct {
	callsLog *[]string
	command  PersistDocumentObservationCommand
	result   PersistDocumentVersionResult
	err      error
	calls    int
}

func (persister *documentObservationPersisterFake) PersistDocumentObservation(_ context.Context, command PersistDocumentObservationCommand) (PersistDocumentVersionResult, error) {
	*persister.callsLog = append(*persister.callsLog, "persist")
	persister.calls++
	persister.command = command
	return persister.result, persister.err
}

type documentProjectionAuthorizationReaderFake struct {
	callsLog *[]string
	query    DocumentProjectionAuthorizationQuery
	result   DocumentProjectionAuthorizationDTO
	err      error
	calls    int
}

func (reader *documentProjectionAuthorizationReaderFake) ReadDocumentProjectionAuthorization(_ context.Context, query DocumentProjectionAuthorizationQuery) (DocumentProjectionAuthorizationDTO, error) {
	*reader.callsLog = append(*reader.callsLog, "authorize")
	reader.calls++
	reader.query = query
	return reader.result, reader.err
}

type documentArtifactProjectorFake struct {
	callsLog *[]string
	commands []ProjectDocumentCommand
	results  map[string]ProjectDocumentResult
	errors   map[string]error
	calls    int
}

func (projector *documentArtifactProjectorFake) Project(_ context.Context, command ProjectDocumentCommand) (ProjectDocumentResult, error) {
	*projector.callsLog = append(*projector.callsLog, "project_"+command.ArtifactType)
	projector.calls++
	projector.commands = append(projector.commands, command)
	return projector.results[command.ArtifactType], projector.errors[command.ArtifactType]
}

type documentSearchProjectionPersisterFake struct {
	callsLog *[]string
	command  PersistDocumentSearchProjectionCommand
	result   DocumentSearchProjectionResult
	err      error
	calls    int
}

func (persister *documentSearchProjectionPersisterFake) PersistSearchProjection(_ context.Context, command PersistDocumentSearchProjectionCommand) (DocumentSearchProjectionResult, error) {
	*persister.callsLog = append(*persister.callsLog, "index")
	persister.calls++
	persister.command = command
	return persister.result, persister.err
}
