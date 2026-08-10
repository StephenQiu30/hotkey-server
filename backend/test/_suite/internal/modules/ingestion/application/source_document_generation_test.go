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
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,structure,index,family,embed,schedule_matches,project_markdown"; got != want {
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
		!reflect.DeepEqual(search.EntityKeys, []string{"café", "café launch"}) ||
		!reflect.DeepEqual(search.ActionKeys, []string{"launch"}) ||
		!reflect.DeepEqual(search.LocationKeys, []string{"san francisco"}) ||
		!reflect.DeepEqual(search.RegionKeys, []string{"us"}) ||
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
	if fixture.contentFamily.calls != 1 || fixture.contentFamily.command.DocumentVersionID != 29 ||
		fixture.contentFamily.command.DerivedArtifactID != 38 || fixture.contentFamily.command.CanonicalPlaintext != "Café launch" ||
		fixture.contentFamily.command.StoreDerivedRightsDecisionID != 81 || fixture.contentFamily.command.RetainRightsDecisionID != 82 ||
		result.ContentFamilyAvailability != SourceDocumentAvailable || result.ContentFamilyDecision == nil || result.ContentFamilyDecision.FamilyID != 56 {
		t.Fatalf("content family assignment = calls %d command %#v result %#v", fixture.contentFamily.calls, fixture.contentFamily.command, result)
	}
	if fixture.matchScheduler.calls != 1 || fixture.matchScheduler.command.DocumentVersionID != 29 ||
		fixture.matchScheduler.result.DocumentVersionID != 29 || fixture.matchScheduler.result.JobID != 59 {
		t.Fatalf("published match scheduling = calls %d command %#v result %#v", fixture.matchScheduler.calls, fixture.matchScheduler.command, fixture.matchScheduler.result)
	}
	if fixture.embedding.calls != 1 || fixture.embedding.command.DocumentVersionID != 29 ||
		fixture.embedding.command.EmbedLocalRightsDecisionID != 84 || fixture.embedding.command.RetainRightsDecisionID != 82 ||
		fixture.embedding.command.NormalizedTextSHA256 != fixture.extraction.PlaintextSHA256 || fixture.embedding.command.Plaintext != "Café launch" ||
		result.EmbeddingAvailability != SourceDocumentAvailable || result.EmbeddingReceipt == nil || result.EmbeddingReceipt.EmbeddingID != 69 {
		t.Fatalf("document embedding = calls %d command %#v result %#v", fixture.embedding.calls, fixture.embedding.command, result)
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
	if fixture.persister.command.Observation.Body != "" || fixture.authorization.calls != 0 || fixture.projector.calls != 0 || fixture.structure.calls != 0 || fixture.search.calls != 0 || fixture.contentFamily.calls != 0 || fixture.embedding.calls != 0 || fixture.matchScheduler.calls != 0 {
		t.Fatalf("metadata-only side effects = persist %#v, auth %d, project %d, search %d, embedding %d, match schedule %d", fixture.persister.command, fixture.authorization.calls, fixture.projector.calls, fixture.search.calls, fixture.embedding.calls, fixture.matchScheduler.calls)
	}
	if result.PlaintextAvailability != SourceDocumentNotApplicable || result.MarkdownAvailability != SourceDocumentNotApplicable ||
		result.SearchAvailability != SourceDocumentNotApplicable || result.PlaintextArtifact != nil || result.MarkdownArtifact != nil ||
		result.SearchProjection != nil || result.ContentFamilyAvailability != SourceDocumentNotApplicable || result.ContentFamilyDecision != nil || result.ContentSHA256 != "" || result.DocumentVersionID != 29 ||
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
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,structure,index"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if result.PlaintextAvailability != SourceDocumentAvailable || result.PlaintextArtifact == nil || result.PlaintextArtifact.ID != 38 ||
		result.SearchAvailability != SourceDocumentUnavailable || result.SearchProjection != nil ||
		result.MarkdownAvailability != SourceDocumentUnavailable || result.MarkdownArtifact != nil ||
		result.LastVerifiedDocumentLifecycleState != DocumentDerivedAvailable || fixture.projector.calls != 1 {
		t.Fatalf("partial search failure result = %#v", result)
	}
}

func TestSourceDocumentGenerationFailsClosedBeforeIndexWhenStructureExtractionFails(t *testing.T) {
	t.Parallel()

	t.Run("extractor unavailable", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.structure.err = sharedrepository.ErrUnavailable
		result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrUnavailable) || fixture.structure.calls != 1 || fixture.search.calls != 0 || fixture.embedding.calls != 0 ||
			result.PlaintextAvailability != SourceDocumentAvailable || result.SearchAvailability != SourceDocumentUnavailable {
			t.Fatalf("structure failure result/error = %#v / %v", result, err)
		}
	})

	t.Run("extractor changes document identity", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.structure.result.DocumentVersionID++
		result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrConflict) || fixture.structure.calls != 1 || fixture.search.calls != 0 ||
			result.PlaintextAvailability != SourceDocumentAvailable || result.SearchAvailability != SourceDocumentUnavailable {
			t.Fatalf("structure mismatch result/error = %#v / %v", result, err)
		}
	})
}

func TestSourceDocumentGenerationReturnsHonestPartialResultWhenMarkdownProjectionFails(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.projector.errors[DocumentProjectionMarkdown] = sharedrepository.ErrUnavailable
	result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want Markdown unavailable", err)
	}
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,structure,index,family,embed,schedule_matches,project_markdown"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if result.PlaintextAvailability != SourceDocumentAvailable || result.SearchAvailability != SourceDocumentAvailable ||
		result.SearchProjection == nil || result.MarkdownAvailability != SourceDocumentUnavailable || result.MarkdownArtifact != nil ||
		result.LastVerifiedDocumentLifecycleState != DocumentDerivedAvailable {
		t.Fatalf("partial Markdown failure result = %#v", result)
	}
}

func TestSourceDocumentGenerationReturnsHonestSearchReceiptWhenPublishedMatchSchedulingFails(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	fixture.matchScheduler.err = sharedrepository.ErrUnavailable
	result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
	if !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want match scheduling unavailable", err)
	}
	if got, want := strings.Join(fixture.calls, ","), "read,extract,persist,authorize,project_plaintext,structure,index,family,embed,schedule_matches"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if result.PlaintextAvailability != SourceDocumentAvailable || result.SearchAvailability != SourceDocumentAvailable ||
		result.SearchProjection == nil || result.MarkdownAvailability != SourceDocumentUnavailable || result.MarkdownArtifact != nil ||
		result.LastVerifiedDocumentLifecycleState != DocumentDerivedAvailable {
		t.Fatalf("partial match scheduling failure result = %#v", result)
	}
}

func TestSourceDocumentGenerationDoesNotScheduleMatchUntilEmbeddingAttemptIsTerminal(t *testing.T) {
	t.Parallel()

	t.Run("transient embedding failure", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.embedding.err = sharedrepository.ErrUnavailable
		result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if !errors.Is(err, sharedrepository.ErrUnavailable) || fixture.matchScheduler.calls != 0 ||
			result.SearchAvailability != SourceDocumentAvailable || result.EmbeddingAvailability != SourceDocumentUnavailable {
			t.Fatalf("embedding failure result/error/match calls = %#v / %v / %d", result, err, fixture.matchScheduler.calls)
		}
	})

	t.Run("explicit model degradation", func(t *testing.T) {
		fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
		fixture.embedding.result = ProduceDocumentEmbeddingResult{
			DocumentVersionID: 29, Availability: SourceDocumentUnavailable, UnavailableReason: "embedding_model_unavailable",
		}
		result, err := fixture.service.Generate(context.Background(), GenerateSourceDocumentCommand{EvidenceReferenceID: 71})
		if err != nil || fixture.matchScheduler.calls != 1 || result.EmbeddingAvailability != SourceDocumentUnavailable ||
			result.EmbeddingUnavailableReason != "embedding_model_unavailable" || result.EmbeddingReceipt != nil {
			t.Fatalf("degraded embedding result/error/match calls = %#v / %v / %d", result, err, fixture.matchScheduler.calls)
		}
	})
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

func TestSelectedSourceEvidenceAcceptsUntitledPlatformComment(t *testing.T) {
	t.Parallel()

	fixture := newSourceDocumentGenerationFixture(t, BodyCompletenessFull)
	evidence := fixture.reader.evidence
	evidence.SourceCode = "hacker_news"
	evidence.ContentType = "comment"
	evidence.Title = ""
	evidence.BodyOrigin = BodyOriginPlatformPost
	evidence.SelectedPayload = []byte(`{"id":2921983,"type":"comment","text":"An exact archived comment"}`)
	digest := sha256.Sum256(evidence.SelectedPayload)
	evidence.SelectedPayloadSHA256 = fmt.Sprintf("%x", digest)
	evidence.PayloadMIMEType = "application/json"
	evidence.SelectorVersion = "whole-payload-sha256-v1"

	if err := validateSelectedSourceEvidence(evidence, evidence.EvidenceReferenceID); err != nil {
		t.Fatalf("untitled platform comment was rejected: %v", err)
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
	service        *SourceDocumentGenerationService
	reader         *selectedSourceEvidenceReaderFake
	extractor      *selectedSourceBodyExtractorFake
	persister      *documentObservationPersisterFake
	authorization  *documentProjectionAuthorizationReaderFake
	projector      *documentArtifactProjectorFake
	search         *documentSearchProjectionPersisterFake
	contentFamily  *documentContentFamilyAssignerFake
	structure      *documentStructureExtractorFake
	embedding      *documentEmbeddingProducerFake
	matchScheduler *publishedDocumentMatchEvaluationSchedulerFake
	extraction     ExtractSelectedSourceBodyResult
	calls          []string
	now            time.Time
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
	addSourceDocumentExtractionAnchorFacts(&fixture.extraction)
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
	embedID := int64(84)
	fixture.authorization = &documentProjectionAuthorizationReaderFake{callsLog: &fixture.calls, result: DocumentProjectionAuthorizationDTO{
		SourceConnectionID: 7, DocumentVersionID: 29, ContentSHA256: fixture.extraction.PlaintextSHA256,
		DecisionAt: now, StoreDerivedRightsDecisionID: 81, RetainRightsDecisionID: 82,
		DisplayPrivateRightsDecisionID: &displayID, EmbedLocalRightsDecisionID: &embedID,
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
				AnchorMap: &DerivedArtifactAnchorMapDTO{
					NormalizationVersion:    fixture.extraction.TextNormalizationVersion,
					AnchorMapProfileVersion: fixture.extraction.AnchorMapProfileVersion,
					PlaintextSHA256:         fixture.extraction.PlaintextSHA256, MarkdownSHA256: fixture.extraction.MarkdownSHA256,
					AnchorMapSHA256: fixture.extraction.AnchorMapSHA256,
				},
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
	fixture.structure = &documentStructureExtractorFake{callsLog: &fixture.calls, result: ExtractDocumentStructureResult{
		DocumentVersionID: 29, ContentSHA256: fixture.extraction.PlaintextSHA256,
		ProfileVersion: CanonicalDocumentStructureProfileVersion,
		EntityKeys:     []string{"café", "café launch"}, ActionKeys: []string{"launch"},
		LocationKeys: []string{"san francisco"}, RegionKeys: []string{"us"},
	}}
	fixture.contentFamily = &documentContentFamilyAssignerFake{callsLog: &fixture.calls, result: AssignDocumentContentFamilyResult{Decision: ContentFamilyDecisionDTO{
		DecisionID: 55, FamilyID: 56, FamilyVersion: 1, DocumentVersionID: 29, RootDocumentVersionID: 29,
		Action: "create", Relation: "unrelated", DecisionProfileVersion: CanonicalContentFamilyDecisionProfileVersion,
		ReasonCodes: []string{"no_candidate"},
	}}}
	fixture.matchScheduler = &publishedDocumentMatchEvaluationSchedulerFake{callsLog: &fixture.calls, result: SchedulePublishedDocumentMatchEvaluationResult{
		DocumentVersionID: 29, JobID: 59, Created: true,
	}}
	fixture.embedding = &documentEmbeddingProducerFake{callsLog: &fixture.calls, result: ProduceDocumentEmbeddingResult{
		DocumentVersionID: 29, Availability: SourceDocumentAvailable,
		Receipt: &DocumentEmbeddingReceiptResult{
			EmbeddingID: 69, DocumentVersionID: 29, SourceConnectionID: 7, EmbedLocalRightsDecisionID: 84,
			RetainRightsDecisionID: 82, ModelProfileID: 91, ModelProfileVersion: 1,
			ModelVersion: "document-embedding-v1", NormalizedTextSHA256: fixture.extraction.PlaintextSHA256,
			AIRunID: 101, RetentionUntil: now.Add(30 * 24 * time.Hour), CreatedAt: now,
			LifecycleState: RecallAssetLifecycleActive, Created: true,
		},
	}}
	service, err := NewSourceDocumentGenerationService(SourceDocumentGenerationDependencies{
		Evidence: fixture.reader, Extractor: fixture.extractor, DocumentVersions: fixture.persister,
		Authorizations: fixture.authorization, Projections: fixture.projector, SearchProjections: fixture.search,
		ContentFamilies:           fixture.contentFamily,
		StructureExtractor:        fixture.structure,
		PublishedMatchEvaluations: fixture.matchScheduler,
		DocumentEmbeddings:        fixture.embedding,
		Now:                       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSourceDocumentGenerationService() error = %v", err)
	}
	fixture.service = service
	return fixture
}

func addSourceDocumentExtractionAnchorFacts(extraction *ExtractSelectedSourceBodyResult) {
	if extraction == nil || extraction.Plaintext == "" || extraction.Markdown == "" {
		return
	}
	extraction.MarkdownSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(extraction.Markdown)))
	extraction.TextNormalizationVersion = CanonicalDocumentTextNormalizationVersion
	extraction.AnchorMapProfileVersion = CanonicalDocumentAnchorMapProfileVersion
	extraction.AnchorBlocks = []DocumentAnchorBlockDTO{{
		Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: int64(len(extraction.Plaintext)),
		MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: int64(len(extraction.Markdown)),
		MarkdownAnchor: DocumentMarkdownAnchor(0, extraction.Plaintext),
	}}
	extraction.AnchorMapSHA256 = DocumentAnchorMapSHA256(MapDocumentTextResult{
		Plaintext: extraction.Plaintext, NormalizationVersion: extraction.TextNormalizationVersion,
		AnchorMapProfileVersion: extraction.AnchorMapProfileVersion, PlaintextSHA256: extraction.PlaintextSHA256,
		MarkdownSHA256: extraction.MarkdownSHA256, Blocks: extraction.AnchorBlocks,
	})
}

type documentStructureExtractorFake struct {
	callsLog *[]string
	command  ExtractDocumentStructureCommand
	result   ExtractDocumentStructureResult
	err      error
	calls    int
}

func (extractor *documentStructureExtractorFake) ExtractDocumentStructure(_ context.Context, command ExtractDocumentStructureCommand) (ExtractDocumentStructureResult, error) {
	*extractor.callsLog = append(*extractor.callsLog, "structure")
	extractor.calls++
	extractor.command = command
	return extractor.result, extractor.err
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

type publishedDocumentMatchEvaluationSchedulerFake struct {
	callsLog *[]string
	command  SchedulePublishedDocumentMatchEvaluationCommand
	result   SchedulePublishedDocumentMatchEvaluationResult
	err      error
	calls    int
}

type documentContentFamilyAssignerFake struct {
	callsLog *[]string
	command  AssignDocumentContentFamilyCommand
	result   AssignDocumentContentFamilyResult
	err      error
	calls    int
}

func (assigner *documentContentFamilyAssignerFake) Assign(_ context.Context, command AssignDocumentContentFamilyCommand) (AssignDocumentContentFamilyResult, error) {
	*assigner.callsLog = append(*assigner.callsLog, "family")
	assigner.calls++
	assigner.command = command
	return assigner.result, assigner.err
}

func (scheduler *publishedDocumentMatchEvaluationSchedulerFake) SchedulePublishedDocumentMatchEvaluation(_ context.Context, command SchedulePublishedDocumentMatchEvaluationCommand) (SchedulePublishedDocumentMatchEvaluationResult, error) {
	*scheduler.callsLog = append(*scheduler.callsLog, "schedule_matches")
	scheduler.calls++
	scheduler.command = command
	return scheduler.result, scheduler.err
}

type documentEmbeddingProducerFake struct {
	callsLog *[]string
	command  ProduceDocumentEmbeddingCommand
	result   ProduceDocumentEmbeddingResult
	err      error
	calls    int
}

func (producer *documentEmbeddingProducerFake) ProduceDocumentEmbedding(_ context.Context, command ProduceDocumentEmbeddingCommand) (ProduceDocumentEmbeddingResult, error) {
	*producer.callsLog = append(*producer.callsLog, "embed")
	producer.calls++
	producer.command = command
	return producer.result, producer.err
}

func (persister *documentSearchProjectionPersisterFake) PersistSearchProjection(_ context.Context, command PersistDocumentSearchProjectionCommand) (DocumentSearchProjectionResult, error) {
	*persister.callsLog = append(*persister.callsLog, "index")
	persister.calls++
	persister.command = command
	return persister.result, persister.err
}
