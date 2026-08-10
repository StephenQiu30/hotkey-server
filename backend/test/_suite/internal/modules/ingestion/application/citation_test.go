package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestCitationServiceReadsOnlyTheRequestedDocumentVersion(t *testing.T) {
	t.Parallel()
	reader := &citationMetadataReaderStub{records: map[int64]CitationReadDTO{
		41: readyCitationReadDTO(41, "old title", strings.Repeat("1", 64), strings.Repeat("a", 64)),
		42: readyCitationReadDTO(42, "new title", strings.Repeat("2", 64), strings.Repeat("b", 64)),
	}}
	projection := &citationProjectionReaderStub{result: knowledgeapplication.DocumentProjectionResultDTO{
		Content: "# old title\n", MIMEType: "text/markdown; charset=utf-8",
		SHA256: strings.Repeat("a", 64), SizeBytes: 12,
	}}
	service, err := NewCitationService(CitationDependencies{Citations: reader, Projections: projection})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatalf("GetCitation() error = %v", err)
	}
	if reader.lastID != 41 || first.Citation.DocumentVersionID != 41 || first.Citation.Title != "old title" ||
		first.Citation.Availability != CitationFullArchive || first.Citation.Artifact == nil || first.Citation.Artifact.AnchorMap == nil ||
		len(first.Citation.Artifact.AnchorMap.Blocks) != 1 || first.Citation.Artifact.AnchorMap.Blocks[0].MarkdownAnchor != "body-0000-000000000000" {
		t.Fatalf("exact citation = %#v, repository id = %d", first, reader.lastID)
	}

	// A newer immutable version exists, but the exact citation remains pinned.
	reader.currentDocumentVersionID = 42
	again, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil || again.Citation.DocumentVersionID != 41 || again.Citation.Title != "old title" {
		t.Fatalf("citation after current-version drift = %#v, %v", again, err)
	}

	document, err := service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if projection.lastQuery.DocumentVersionID != 41 || projection.lastQuery.DocumentID != 11 ||
		document.Citation.DocumentVersionID != 41 || document.Markdown != "# old title\n" ||
		document.ETag != `"`+strings.Repeat("a", 64)+`"` {
		t.Fatalf("exact document = %#v, projection query = %#v", document, projection.lastQuery)
	}
}

func TestCitationServiceFailsClosedForUnprovablePublisherAndLocator(t *testing.T) {
	t.Parallel()
	reader := &citationMetadataReaderStub{records: map[int64]CitationReadDTO{
		41: readyCitationReadDTO(41, "title", strings.Repeat("1", 64), strings.Repeat("a", 64)),
	}}
	service, err := NewCitationService(CitationDependencies{Citations: reader, Projections: &citationProjectionReaderStub{}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatal(err)
	}
	citation := result.Citation
	if citation.Publisher != nil || citation.PublisherAvailability != CitationFactUnavailable || citation.PublisherUnavailableReason != CitationReasonPublisherUnavailable {
		t.Fatalf("publisher projection = %#v, want explicit unavailable", citation)
	}
	if citation.ContentOrigin != nil || citation.ContentOriginAvailability != CitationFactUnavailable ||
		citation.ContentOriginUnavailableReason != CitationReasonContentOriginUnavailable || len(citation.Distributors) != 0 {
		t.Fatalf("origin/distributor projection = %#v/%#v, want explicit unavailable/empty", citation.ContentOrigin, citation.Distributors)
	}
	if citation.LocatorAvailability != CitationFactUnavailable || citation.LocatorUnavailableReason != CitationReasonLocatorUnavailable ||
		citation.ExactQuote != nil || citation.UTF8ByteStart != nil || citation.UTF8ByteEnd != nil || citation.AnchorMap != nil {
		t.Fatalf("locator projection = %#v, want fail-closed unavailable", citation)
	}
	if citation.SourceRecordURL == nil || citation.CanonicalURL == nil || citation.DiscussionURL == nil {
		t.Fatalf("provenance URLs = %#v/%#v/%#v, want all three", citation.SourceRecordURL, citation.CanonicalURL, citation.DiscussionURL)
	}
}

func TestCitationServiceProjectsOnlyExplicitEvidenceBoundParties(t *testing.T) {
	t.Parallel()
	record := readyCitationReadDTO(41, "title", strings.Repeat("1", 64), strings.Repeat("a", 64))
	record.Publisher = &CitationPartyReadDTO{
		Role: "publisher", Kind: "organization", IdentityNamespace: "publisher-registry",
		ExternalID: "publisher-42", DisplayName: "Example Newsroom", HomepageURL: optionalCitationString("https://publisher.example.test/"),
	}
	record.ContentOrigin = &CitationPartyReadDTO{
		Role: "content_origin", Kind: "organization", IdentityNamespace: "origin-registry",
		ExternalID: "origin-9", DisplayName: "Original Desk",
	}
	record.Distributors = []CitationPartyReadDTO{{
		Role: "distributor", Kind: "account", IdentityNamespace: "platform-account",
		ExternalID: "distribution-7", DisplayName: "Syndication Desk", HomepageURL: optionalCitationString("https://distribution.example.test/accounts/7"),
	}}
	service, err := NewCitationService(CitationDependencies{
		Citations:   &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: record}},
		Projections: &citationProjectionReaderStub{},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatal(err)
	}
	citation := result.Citation
	if citation.Publisher == nil || *citation.Publisher != "Example Newsroom" || citation.PublisherParty == nil ||
		citation.PublisherAvailability != CitationFactAvailable || citation.PublisherUnavailableReason != "" {
		t.Fatalf("publisher projection = %#v", citation)
	}
	if citation.ContentOrigin == nil || citation.ContentOrigin.DisplayName != "Original Desk" ||
		citation.ContentOriginAvailability != CitationFactAvailable || len(citation.Distributors) != 1 ||
		citation.Distributors[0].DisplayName != "Syndication Desk" {
		t.Fatalf("origin/distributor projection = %#v/%#v", citation.ContentOrigin, citation.Distributors)
	}

	unsafe := record
	unsafe.Publisher = &CitationPartyReadDTO{
		Role: "publisher", Kind: "organization", IdentityNamespace: "publisher-registry",
		ExternalID: "publisher-42", DisplayName: "Unsafe\nPublisher", HomepageURL: optionalCitationString("javascript:alert(1)"),
	}
	service, err = NewCitationService(CitationDependencies{
		Citations:   &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: unsafe}},
		Projections: &citationProjectionReaderStub{},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatal(err)
	}
	if result.Citation.Publisher != nil || result.Citation.PublisherParty != nil ||
		result.Citation.PublisherAvailability != CitationFactUnavailable {
		t.Fatalf("unsafe publisher was exposed: %#v", result.Citation)
	}
}

func optionalCitationString(value string) *string { return &value }

func TestCitationServiceFiltersUnsafeProvenanceURLs(t *testing.T) {
	t.Parallel()
	safe := "  https://publisher.example.test/articles/41?view=archive  "
	fragment := "https://publisher.example.test/articles/41#private-anchor"
	userinfo := "https://reader:secret@publisher.example.test/articles/41"
	control := "https://publisher.example.test/articles/\n41"
	record := readyCitationReadDTO(41, "title", strings.Repeat("1", 64), strings.Repeat("a", 64))
	record.SourceRecordURL = &safe
	record.CanonicalURL = &fragment
	record.DiscussionURL = &userinfo
	service, err := NewCitationService(CitationDependencies{
		Citations:   &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: record}},
		Projections: &citationProjectionReaderStub{},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatal(err)
	}
	if result.Citation.SourceRecordURL == nil || *result.Citation.SourceRecordURL != strings.TrimSpace(safe) ||
		result.Citation.CanonicalURL != nil || result.Citation.DiscussionURL != nil {
		t.Fatalf("filtered URLs = %#v/%#v/%#v", result.Citation.SourceRecordURL, result.Citation.CanonicalURL, result.Citation.DiscussionURL)
	}
	for _, unsafe := range []*string{&fragment, &userinfo, &control} {
		if safeCitationURL(unsafe) != nil {
			t.Fatalf("safeCitationURL(%q) accepted an unsafe URL", *unsafe)
		}
	}
	tooLong := "https://publisher.example.test/" + strings.Repeat("a", 2049)
	if safeCitationURL(&tooLong) != nil {
		t.Fatal("safeCitationURL() accepted a URL longer than 2048 bytes")
	}
}

func TestCitationServiceClassifiesReadAvailabilityWithoutReadingVault(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		mutate       func(*CitationReadDTO)
		availability CitationAvailability
		reason       CitationUnavailableReason
		kind         DocumentReadFailureKind
	}{
		{name: "document not readable", mutate: func(value *CitationReadDTO) { value.DocumentLifecycleState = DocumentDerivedAvailable }, availability: CitationTemporarilyUnavailable, reason: CitationReasonDocumentNotReadable, kind: DocumentReadFailureNotReadable},
		{name: "policy blocked lifecycle", mutate: func(value *CitationReadDTO) { value.DocumentLifecycleState = DocumentPolicyBlocked }, availability: CitationPolicyBlocked, reason: CitationReasonPolicyBlocked, kind: DocumentReadFailurePolicy},
		{name: "policy blocked observation", mutate: func(value *CitationReadDTO) { value.ObservationState = "policy_blocked" }, availability: CitationPolicyBlocked, reason: CitationReasonPolicyBlocked, kind: DocumentReadFailurePolicy},
		{name: "display permission withdrawn", mutate: func(value *CitationReadDTO) { value.DisplayPrivateAllowed = false }, availability: CitationPolicyBlocked, reason: CitationReasonPermissionDenied, kind: DocumentReadFailurePermission},
		{name: "store derived withdrawn", mutate: func(value *CitationReadDTO) { value.Artifact.StoreDerivedAllowed = false }, availability: CitationPolicyBlocked, reason: CitationReasonPolicyBlocked, kind: DocumentReadFailurePolicy},
		{name: "retain withdrawn", mutate: func(value *CitationReadDTO) { value.Artifact.RetainAllowed = false }, availability: CitationTombstoned, reason: CitationReasonRetentionUnavailable, kind: DocumentReadFailureRetention},
		{name: "retention expired", mutate: func(value *CitationReadDTO) { value.Artifact.RetentionUntil = now }, availability: CitationTombstoned, reason: CitationReasonRetentionUnavailable, kind: DocumentReadFailureRetention},
		{name: "artifact missing", mutate: func(value *CitationReadDTO) { value.Artifact = nil }, availability: CitationTemporarilyUnavailable, reason: CitationReasonArtifactMissing, kind: DocumentReadFailureMissing},
		{name: "artifact failed", mutate: func(value *CitationReadDTO) {
			value.Artifact.LifecycleState = "derive_failed"
			value.Artifact.Active = false
			failure := "vault_write_failed"
			value.Artifact.FailureCode = &failure
		}, availability: CitationQuarantined, reason: CitationReasonIntegrityFailed, kind: DocumentReadFailureIntegrity},
		{name: "artifact mime mismatch", mutate: func(value *CitationReadDTO) { value.Artifact.MIMEType = "application/octet-stream" }, availability: CitationQuarantined, reason: CitationReasonIntegrityFailed, kind: DocumentReadFailureIntegrity},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			record := readyCitationReadDTO(41, "title", strings.Repeat("1", 64), strings.Repeat("a", 64))
			record.RightsEvaluatedAt = now
			record.Artifact.RetentionUntil = now.Add(time.Hour)
			test.mutate(&record)
			projection := &citationProjectionReaderStub{}
			service, err := NewCitationService(CitationDependencies{
				Citations: &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: record}}, Projections: projection,
			})
			if err != nil {
				t.Fatal(err)
			}
			citation, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
			if err != nil || citation.Citation.Availability != test.availability || citation.Citation.UnavailableReason != test.reason {
				t.Fatalf("GetCitation() = %#v, %v, want %s/%s", citation, err, test.availability, test.reason)
			}
			if citation.Citation.Artifact != nil || citation.Citation.ContentSHA256 != nil {
				t.Fatalf("unavailable citation exposed content-derived metadata: %#v", citation.Citation)
			}
			_, err = service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41})
			var readError *DocumentReadError
			if !errors.As(err, &readError) || readError.Kind != test.kind {
				t.Fatalf("GetDocument() error = %#v, want %s", err, test.kind)
			}
			if projection.calls != 0 {
				t.Fatalf("Vault reads = %d, want zero before rights/lifecycle gate", projection.calls)
			}
		})
	}
}

func TestCitationServiceProjectsMetadataOnlyWithoutInventingAnArtifact(t *testing.T) {
	t.Parallel()
	record := readyCitationReadDTO(41, "metadata title", strings.Repeat("1", 64), strings.Repeat("a", 64))
	record.DocumentLifecycleState = DocumentPolicyPending
	record.Completeness = BodyCompletenessMetadataOnly
	record.Artifact = nil
	projection := &citationProjectionReaderStub{}
	service, err := NewCitationService(CitationDependencies{
		Citations: &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: record}}, Projections: projection,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.GetCitation(context.Background(), CitationQuery{DocumentVersionID: 41})
	if err != nil {
		t.Fatal(err)
	}
	if result.Citation.Availability != CitationMetadataOnly || result.Citation.UnavailableReason != CitationReasonNoCitableBody ||
		result.Citation.Artifact != nil || result.Citation.ContentSHA256 != nil {
		t.Fatalf("metadata-only citation = %#v", result.Citation)
	}
	_, err = service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41})
	var readError *DocumentReadError
	if !errors.As(err, &readError) || readError.Kind != DocumentReadFailureNotReadable || projection.calls != 0 {
		t.Fatalf("metadata-only document error/reads = %#v/%d", err, projection.calls)
	}
}

func TestCitationServiceVerifiesProjectionBeforeStrongETag304(t *testing.T) {
	t.Parallel()
	artifactSHA := strings.Repeat("a", 64)
	record := readyCitationReadDTO(41, "title", strings.Repeat("1", 64), artifactSHA)
	projection := &citationProjectionReaderStub{result: knowledgeapplication.DocumentProjectionResultDTO{
		Content: "# document!\n", MIMEType: "text/markdown; charset=utf-8", SHA256: artifactSHA, SizeBytes: 12,
	}}
	service, err := NewCitationService(CitationDependencies{
		Citations: &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: record}}, Projections: projection,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41, IfNoneMatch: `"` + artifactSHA + `"`})
	if err != nil || !result.NotModified || result.ETag != `"`+artifactSHA+`"` || projection.calls != 1 {
		t.Fatalf("matching verified ETag result = %#v, err=%v reads=%d", result, err, projection.calls)
	}

	projection.err = knowledgeapplication.ErrProjectionNotFound
	_, err = service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41, IfNoneMatch: `"` + artifactSHA + `"`})
	var readError *DocumentReadError
	if !errors.As(err, &readError) || readError.Kind != DocumentReadFailureMissing || projection.calls != 2 {
		t.Fatalf("missing projection with matching ETag error = %#v reads=%d, want missing after a read", err, projection.calls)
	}

	projection.err = knowledgeapplication.ErrProjectionIntegrity
	_, err = service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41})
	if !errors.As(err, &readError) || readError.Kind != DocumentReadFailureIntegrity {
		t.Fatalf("integrity error = %#v, want typed integrity", err)
	}
	projection.err = knowledgeapplication.ErrProjectionUnavailable
	_, err = service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41})
	if !errors.As(err, &readError) || readError.Kind != DocumentReadFailureUnavailable {
		t.Fatalf("unavailable error = %#v, want typed unavailable", err)
	}
}

func TestCitationServiceRechecksRightsAfterProjectionRead(t *testing.T) {
	t.Parallel()
	record := readyCitationReadDTO(41, "title", strings.Repeat("1", 64), strings.Repeat("a", 64))
	reader := &citationMetadataReaderStub{records: map[int64]CitationReadDTO{41: record}}
	projection := &citationProjectionReaderStub{result: knowledgeapplication.DocumentProjectionResultDTO{
		Content: "# document!\n", MIMEType: "text/markdown; charset=utf-8",
		SHA256: strings.Repeat("a", 64), SizeBytes: 12,
	}}
	projection.onRead = func() {
		revoked := reader.records[41]
		revoked.DisplayPrivateAllowed = false
		reader.records[41] = revoked
	}
	service, err := NewCitationService(CitationDependencies{Citations: reader, Projections: projection})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41})
	var readError *DocumentReadError
	if !errors.As(err, &readError) || readError.Kind != DocumentReadFailurePermission {
		t.Fatalf("revoke-during-read error = %#v, want permission", err)
	}
	if reader.calls != 2 || projection.calls != 1 {
		t.Fatalf("metadata/projection calls = %d/%d, want pre-read and post-read rights checks", reader.calls, projection.calls)
	}
}

func TestCitationServiceRejectsWeakOrMalformedETagBeforeDependencies(t *testing.T) {
	t.Parallel()
	reader := &citationMetadataReaderStub{}
	projection := &citationProjectionReaderStub{}
	service, err := NewCitationService(CitationDependencies{Citations: reader, Projections: projection})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"W/\"" + strings.Repeat("a", 64) + "\"", strings.Repeat("a", 64), `"abc"`, `"` + strings.Repeat("a", 64) + `", "` + strings.Repeat("b", 64) + `"`, "*"} {
		_, err := service.GetDocument(context.Background(), DocumentQuery{DocumentVersionID: 41, IfNoneMatch: value})
		if !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("If-None-Match %q error = %v, want invalid input", value, err)
		}
	}
	if reader.calls != 0 || projection.calls != 0 {
		t.Fatalf("dependency calls = metadata %d / projection %d, want zero", reader.calls, projection.calls)
	}
}

func readyCitationReadDTO(documentVersionID int64, title, contentSHA, artifactSHA string) CitationReadDTO {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	author := "Ada"
	sourceRecordURL := "https://feed.example.test/items/41"
	canonicalURL := "https://publisher.example.test/articles/41"
	discussionURL := "https://forum.example.test/discussions/41"
	availableAt := now.Add(-time.Minute)
	retentionDays := 30
	anchorBlocks := []DocumentAnchorBlockDTO{{
		Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: 1,
		MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: 12, MarkdownAnchor: "body-0000-000000000000",
	}}
	anchorIdentity := DerivedArtifactAnchorMapDTO{
		NormalizationVersion:    CanonicalDocumentTextNormalizationVersion,
		AnchorMapProfileVersion: CanonicalDocumentAnchorMapProfileVersion,
		PlaintextSHA256:         contentSHA, MarkdownSHA256: artifactSHA,
	}
	anchorIdentity.AnchorMapSHA256 = DocumentAnchorMapSHA256(MapDocumentTextResult{
		NormalizationVersion: anchorIdentity.NormalizationVersion, AnchorMapProfileVersion: anchorIdentity.AnchorMapProfileVersion,
		PlaintextSHA256: anchorIdentity.PlaintextSHA256, MarkdownSHA256: anchorIdentity.MarkdownSHA256, Blocks: anchorBlocks,
	})
	return CitationReadDTO{
		DocumentID: 11, DocumentVersionID: documentVersionID, SourceConnectionID: 7,
		DocumentState: "active", DocumentLifecycleState: DocumentReadable,
		ObservationState: "active",
		SourceType:       "rss", SourceName: "Product feed", Title: title, Author: &author,
		SourceRecordURL: &sourceRecordURL, CanonicalURL: &canonicalURL, DiscussionURL: &discussionURL,
		BodyOrigin: BodyOriginFeedContent, Completeness: BodyCompletenessFull,
		Language: "en", PublishedAt: &now, CapturedAt: now.Add(-time.Hour), ContentSHA256: contentSHA,
		DisplayPrivateAllowed: true, RightsEvaluatedAt: now,
		Artifact: &CitationArtifactReadDTO{
			ArtifactType: "markdown", TransformerProfileSHA256: strings.Repeat("b", 64),
			MIMEType: "text/markdown; charset=utf-8", SHA256: artifactSHA, SizeBytes: 12,
			LifecycleState: "derived_available", Active: true, AvailableAt: &availableAt,
			RetentionUntil: now.Add(24 * time.Hour), StoreDerivedAllowed: true, RetainAllowed: true,
			CurrentRetentionDays: &retentionDays,
			AnchorMap: &CitationArtifactAnchorMapReadDTO{
				NormalizationVersion: anchorIdentity.NormalizationVersion, AnchorMapProfileVersion: anchorIdentity.AnchorMapProfileVersion,
				PlaintextSHA256: anchorIdentity.PlaintextSHA256, MarkdownSHA256: anchorIdentity.MarkdownSHA256,
				AnchorMapSHA256: anchorIdentity.AnchorMapSHA256,
				Blocks: []CitationAnchorBlockReadDTO{{
					Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: 1,
					MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: 12, MarkdownAnchor: "body-0000-000000000000",
				}},
			},
		},
	}
}

type citationMetadataReaderStub struct {
	records                  map[int64]CitationReadDTO
	err                      error
	lastID                   int64
	calls                    int
	currentDocumentVersionID int64
}

func (reader *citationMetadataReaderStub) ReadCitation(_ context.Context, documentVersionID int64) (CitationReadDTO, error) {
	reader.calls++
	reader.lastID = documentVersionID
	if reader.err != nil {
		return CitationReadDTO{}, reader.err
	}
	value, found := reader.records[documentVersionID]
	if !found {
		return CitationReadDTO{}, sharedrepository.ErrNotFound
	}
	return value, nil
}

type citationProjectionReaderStub struct {
	result    knowledgeapplication.DocumentProjectionResultDTO
	err       error
	lastQuery knowledgeapplication.DocumentProjectionQueryDTO
	calls     int
	onRead    func()
}

func (reader *citationProjectionReaderStub) ReadDocumentProjection(_ context.Context, query knowledgeapplication.DocumentProjectionQueryDTO) (knowledgeapplication.DocumentProjectionResultDTO, error) {
	reader.calls++
	reader.lastQuery = query
	if reader.onRead != nil {
		reader.onRead()
	}
	return reader.result, reader.err
}
