package application

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const citationDocumentMaximumBytes int64 = 4 << 20

type CitationAvailability string

const (
	CitationFullArchive            CitationAvailability = "full_archive"
	CitationPartialArchive         CitationAvailability = "partial_archive"
	CitationSummaryOnly            CitationAvailability = "summary_only"
	CitationMetadataOnly           CitationAvailability = "metadata_only"
	CitationPolicyBlocked          CitationAvailability = "policy_blocked"
	CitationTemporarilyUnavailable CitationAvailability = "temporarily_unavailable"
	CitationQuarantined            CitationAvailability = "quarantined"
	CitationTombstoned             CitationAvailability = "tombstoned"
)

type CitationFactAvailability string

const (
	CitationFactAvailable   CitationFactAvailability = "available"
	CitationFactUnavailable CitationFactAvailability = "unavailable"
)

type CitationUnavailableReason string

const (
	CitationReasonDocumentNotReadable      CitationUnavailableReason = "document_not_readable"
	CitationReasonPolicyBlocked            CitationUnavailableReason = "policy_blocked"
	CitationReasonPermissionDenied         CitationUnavailableReason = "permission_denied"
	CitationReasonRetentionUnavailable     CitationUnavailableReason = "retention_unavailable"
	CitationReasonArtifactMissing          CitationUnavailableReason = "artifact_missing"
	CitationReasonIntegrityFailed          CitationUnavailableReason = "integrity_failed"
	CitationReasonSourceUnavailable        CitationUnavailableReason = "source_unavailable"
	CitationReasonNoCitableBody            CitationUnavailableReason = "no_citable_body"
	CitationReasonPublisherUnavailable     CitationUnavailableReason = "publisher_unavailable"
	CitationReasonContentOriginUnavailable CitationUnavailableReason = "content_origin_unavailable"
	CitationReasonLocatorUnavailable       CitationUnavailableReason = "locator_unavailable"
)

type CitationQuery struct {
	DocumentVersionID int64
}

type DocumentQuery struct {
	DocumentVersionID int64
	IfNoneMatch       string
}

type CitationResult struct {
	Citation CitationDTO
}

type DocumentResult struct {
	Citation    CitationDTO
	Markdown    string
	ETag        string
	NotModified bool
}

// CitationAnchorMapDTO is intentionally absent until an immutable plaintext
// to Markdown anchor fact exists. A nil value is different from an empty map.
type CitationAnchorMapDTO struct {
	NormalizationVersion string
	AnchorMapVersion     string
	MarkdownAnchor       string
}

// CitationArtifactDTO is the public Application projection of an immutable
// derived artifact. It has no Vault path or provider storage identity.
type CitationArtifactDTO struct {
	ArtifactType             string
	TransformerProfileSHA256 string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
	ETag                     string
	AnchorMap                *CitationArtifactAnchorMapDTO
}

type CitationArtifactAnchorBlockDTO struct {
	Ordinal        int
	MarkdownAnchor string
}

type CitationArtifactAnchorMapDTO struct {
	NormalizationVersion    string
	AnchorMapProfileVersion string
	AnchorMapSHA256         string
	Blocks                  []CitationArtifactAnchorBlockDTO
}

type CitationPartyDTO struct {
	Role              string
	Kind              string
	IdentityNamespace string
	ExternalID        string
	DisplayName       string
	HomepageURL       *string
}

// CitationDTO is the explicit safe citation allowlist for one immutable
// DocumentVersion. Unprovable publisher and locator facts remain nil and have
// independent availability reasons.
type CitationDTO struct {
	DocumentID        int64
	DocumentVersionID int64
	SourceType        string
	SourceName        string
	Title             string
	Author            *string
	Publisher         *string
	PublisherParty    *CitationPartyDTO
	ContentOrigin     *CitationPartyDTO
	Distributors      []CitationPartyDTO

	PublisherAvailability          CitationFactAvailability
	PublisherUnavailableReason     CitationUnavailableReason
	ContentOriginAvailability      CitationFactAvailability
	ContentOriginUnavailableReason CitationUnavailableReason
	SourceRecordURL                *string
	CanonicalURL                   *string
	DiscussionURL                  *string

	BodyOrigin    string
	Completeness  string
	Language      string
	PublishedAt   *time.Time
	CapturedAt    time.Time
	ContentSHA256 *string

	Availability      CitationAvailability
	UnavailableReason CitationUnavailableReason
	Artifact          *CitationArtifactDTO

	LocatorAvailability      CitationFactAvailability
	LocatorUnavailableReason CitationUnavailableReason
	ExactQuote               *string
	UTF8ByteStart            *int64
	UTF8ByteEnd              *int64
	AnchorMap                *CitationAnchorMapDTO
}

// CitationArtifactReadDTO is an Application read model, not a persistence
// record. Rights booleans are evaluated by the database in the same statement
// that selects the exact artifact.
type CitationArtifactReadDTO struct {
	ArtifactType             string
	TransformerProfileSHA256 string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
	LifecycleState           string
	Active                   bool
	FailureCode              *string
	AvailableAt              *time.Time
	RetentionUntil           time.Time
	StoreDerivedAllowed      bool
	RetainAllowed            bool
	CurrentRetentionDays     *int
	AnchorMap                *CitationArtifactAnchorMapReadDTO
}

type CitationAnchorBlockReadDTO struct {
	Ordinal                int
	PlaintextUTF8ByteStart int64
	PlaintextUTF8ByteEnd   int64
	MarkdownUTF8ByteStart  int64
	MarkdownUTF8ByteEnd    int64
	MarkdownAnchor         string
}

type CitationArtifactAnchorMapReadDTO struct {
	NormalizationVersion    string
	AnchorMapProfileVersion string
	PlaintextSHA256         string
	MarkdownSHA256          string
	AnchorMapSHA256         string
	Blocks                  []CitationAnchorBlockReadDTO
}

type CitationPartyReadDTO struct {
	Role              string
	Kind              string
	IdentityNamespace string
	ExternalID        string
	DisplayName       string
	HomepageURL       *string
}

// CitationReadDTO is the factual Application projection returned by a
// repository. It deliberately has no raw payload, object key, or Vault path.
type CitationReadDTO struct {
	DocumentID             int64
	DocumentVersionID      int64
	SourceConnectionID     int64
	DocumentState          string
	DocumentLifecycleState string
	ObservationState       string

	SourceType      string
	SourceName      string
	Title           string
	Author          *string
	Publisher       *CitationPartyReadDTO
	ContentOrigin   *CitationPartyReadDTO
	Distributors    []CitationPartyReadDTO
	SourceRecordURL *string
	CanonicalURL    *string
	DiscussionURL   *string

	BodyOrigin    string
	Completeness  string
	Language      string
	PublishedAt   *time.Time
	CapturedAt    time.Time
	ContentSHA256 string

	DisplayPrivateAllowed bool
	RightsEvaluatedAt     time.Time
	Artifact              *CitationArtifactReadDTO
}

type CitationReader interface {
	ReadCitation(context.Context, int64) (CitationReadDTO, error)
}

type CitationUseCases interface {
	GetCitation(context.Context, CitationQuery) (CitationResult, error)
	GetDocument(context.Context, DocumentQuery) (DocumentResult, error)
}

type CitationDependencies struct {
	Citations   CitationReader
	Projections knowledgeapplication.DocumentProjectionReader
}

type CitationService struct {
	citations   CitationReader
	projections knowledgeapplication.DocumentProjectionReader
}

var _ CitationUseCases = (*CitationService)(nil)

func NewCitationService(dependencies CitationDependencies) (*CitationService, error) {
	if dependencies.Citations == nil || dependencies.Projections == nil {
		return nil, errors.New("citation application dependencies are required")
	}
	return &CitationService{citations: dependencies.Citations, projections: dependencies.Projections}, nil
}

func (service *CitationService) GetCitation(ctx context.Context, query CitationQuery) (CitationResult, error) {
	if service == nil || service.citations == nil || service.projections == nil || query.DocumentVersionID <= 0 {
		return CitationResult{}, fmt.Errorf("%w: invalid citation query", sharedrepository.ErrInvalidInput)
	}
	read, err := service.citations.ReadCitation(ctx, query.DocumentVersionID)
	if err != nil {
		return CitationResult{}, fmt.Errorf("read exact document citation: %w", err)
	}
	if read.DocumentVersionID != query.DocumentVersionID || read.DocumentID <= 0 || read.SourceConnectionID <= 0 || read.RightsEvaluatedAt.IsZero() {
		return CitationResult{}, newDocumentReadError(DocumentReadFailureIntegrity, nil)
	}
	return CitationResult{Citation: citationDTO(read)}, nil
}

func (service *CitationService) GetDocument(ctx context.Context, query DocumentQuery) (DocumentResult, error) {
	if query.DocumentVersionID <= 0 || !validCitationIfNoneMatch(query.IfNoneMatch) {
		return DocumentResult{}, fmt.Errorf("%w: invalid document query", sharedrepository.ErrInvalidInput)
	}
	citationResult, err := service.GetCitation(ctx, CitationQuery{DocumentVersionID: query.DocumentVersionID})
	if err != nil {
		return DocumentResult{}, err
	}
	citation := citationResult.Citation
	if !citationArchiveAvailable(citation.Availability) || citation.Artifact == nil {
		return DocumentResult{}, newDocumentReadError(documentReadFailureForCitation(citation.UnavailableReason), nil)
	}
	artifact := citation.Artifact
	read, err := service.projections.ReadDocumentProjection(ctx, knowledgeapplication.DocumentProjectionQueryDTO{
		DocumentID: citation.DocumentID, DocumentVersionID: citation.DocumentVersionID,
		ArtifactType: artifact.ArtifactType, TransformerProfileSHA256: artifact.TransformerProfileSHA256,
		SHA256: artifact.SHA256, SizeBytes: artifact.SizeBytes, MaxBytes: citationDocumentMaximumBytes,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return DocumentResult{}, err
		}
		if errors.Is(err, knowledgeapplication.ErrProjectionNotFound) {
			return DocumentResult{}, newDocumentReadError(DocumentReadFailureMissing, err)
		}
		if errors.Is(err, knowledgeapplication.ErrProjectionUnavailable) {
			return DocumentResult{}, newDocumentReadError(DocumentReadFailureUnavailable, err)
		}
		return DocumentResult{}, newDocumentReadError(DocumentReadFailureIntegrity, err)
	}
	if read.MIMEType != artifact.MIMEType || read.SHA256 != artifact.SHA256 || read.SizeBytes != artifact.SizeBytes ||
		int64(len([]byte(read.Content))) != artifact.SizeBytes || read.Content == "" {
		return DocumentResult{}, newDocumentReadError(DocumentReadFailureIntegrity, knowledgeapplication.ErrProjectionIntegrity)
	}
	// Re-evaluate the current terminal rights set after the bounded Vault read.
	// This prevents a revoke or active-artifact switch between reservation and
	// response from serving bytes authorized only by the earlier statement.
	revalidated, err := service.GetCitation(ctx, CitationQuery{DocumentVersionID: query.DocumentVersionID})
	if err != nil {
		return DocumentResult{}, err
	}
	if !citationArchiveAvailable(revalidated.Citation.Availability) || revalidated.Citation.Artifact == nil {
		return DocumentResult{}, newDocumentReadError(documentReadFailureForCitation(revalidated.Citation.UnavailableReason), nil)
	}
	if !sameCitationArtifact(citation, revalidated.Citation) {
		return DocumentResult{}, newDocumentReadError(DocumentReadFailureNotReadable, nil)
	}
	artifact = revalidated.Citation.Artifact
	result := DocumentResult{Citation: revalidated.Citation, Markdown: read.Content, ETag: artifact.ETag}
	if query.IfNoneMatch != "" && query.IfNoneMatch == artifact.ETag {
		result.Markdown = ""
		result.NotModified = true
	}
	return result, nil
}

func sameCitationArtifact(before, after CitationDTO) bool {
	if before.DocumentID != after.DocumentID || before.DocumentVersionID != after.DocumentVersionID ||
		!sameCitationOptionalString(before.ContentSHA256, after.ContentSHA256) || before.Artifact == nil || after.Artifact == nil {
		return false
	}
	return before.Artifact.ArtifactType == after.Artifact.ArtifactType &&
		before.Artifact.TransformerProfileSHA256 == after.Artifact.TransformerProfileSHA256 &&
		before.Artifact.MIMEType == after.Artifact.MIMEType && before.Artifact.SHA256 == after.Artifact.SHA256 &&
		before.Artifact.SizeBytes == after.Artifact.SizeBytes && before.Artifact.ETag == after.Artifact.ETag &&
		sameCitationArtifactAnchorMap(before.Artifact.AnchorMap, after.Artifact.AnchorMap)
}

func sameCitationArtifactAnchorMap(left, right *CitationArtifactAnchorMapDTO) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left.NormalizationVersion != right.NormalizationVersion || left.AnchorMapProfileVersion != right.AnchorMapProfileVersion ||
		left.AnchorMapSHA256 != right.AnchorMapSHA256 || len(left.Blocks) != len(right.Blocks) {
		return false
	}
	for index := range left.Blocks {
		if left.Blocks[index] != right.Blocks[index] {
			return false
		}
	}
	return true
}

func sameCitationOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func citationDTO(read CitationReadDTO) CitationDTO {
	availability, reason := citationReadAvailability(read)
	publisher := citationPartyDTO(read.Publisher, "publisher")
	contentOrigin := citationPartyDTO(read.ContentOrigin, "content_origin")
	distributors := citationPartyDTOs(read.Distributors, "distributor")
	citation := CitationDTO{
		DocumentID: read.DocumentID, DocumentVersionID: read.DocumentVersionID,
		SourceType: read.SourceType, SourceName: read.SourceName, Title: read.Title,
		Author: safeCitationText(read.Author), PublisherParty: publisher, ContentOrigin: contentOrigin, Distributors: distributors,
		PublisherAvailability: CitationFactUnavailable, PublisherUnavailableReason: CitationReasonPublisherUnavailable,
		ContentOriginAvailability: CitationFactUnavailable, ContentOriginUnavailableReason: CitationReasonContentOriginUnavailable,
		SourceRecordURL: safeCitationURL(read.SourceRecordURL), CanonicalURL: safeCitationURL(read.CanonicalURL),
		DiscussionURL: safeCitationURL(read.DiscussionURL), BodyOrigin: read.BodyOrigin, Completeness: read.Completeness,
		Language: read.Language, PublishedAt: cloneCitationTime(read.PublishedAt), CapturedAt: read.CapturedAt.UTC(),
		Availability: availability, UnavailableReason: reason,
		LocatorAvailability: CitationFactUnavailable, LocatorUnavailableReason: CitationReasonLocatorUnavailable,
	}
	if publisher != nil {
		publisherName := publisher.DisplayName
		citation.Publisher = &publisherName
		citation.PublisherAvailability = CitationFactAvailable
		citation.PublisherUnavailableReason = ""
	}
	if contentOrigin != nil {
		citation.ContentOriginAvailability = CitationFactAvailable
		citation.ContentOriginUnavailableReason = ""
	}
	if citationArchiveAvailable(availability) && read.Artifact != nil {
		contentSHA256 := read.ContentSHA256
		citation.ContentSHA256 = &contentSHA256
		citation.Artifact = &CitationArtifactDTO{
			ArtifactType: read.Artifact.ArtifactType, TransformerProfileSHA256: read.Artifact.TransformerProfileSHA256,
			MIMEType: read.Artifact.MIMEType, SHA256: read.Artifact.SHA256, SizeBytes: read.Artifact.SizeBytes,
			ETag: `"` + read.Artifact.SHA256 + `"`,
		}
		if read.Artifact.AnchorMap != nil {
			citation.Artifact.AnchorMap = citationArtifactAnchorMapDTO(read.Artifact.AnchorMap)
		}
	}
	return citation
}

func citationPartyDTO(value *CitationPartyReadDTO, expectedRole string) *CitationPartyDTO {
	if value == nil || value.Role != expectedRole || !validCitationPartyKind(value.Kind) ||
		!validCitationPartyNamespace(value.IdentityNamespace) ||
		!validCitationPartyText(value.ExternalID, 512) || !validCitationPartyText(value.DisplayName, 512) {
		return nil
	}
	homepage := safeCitationURL(value.HomepageURL)
	if value.HomepageURL != nil && homepage == nil {
		return nil
	}
	return &CitationPartyDTO{
		Role: value.Role, Kind: value.Kind, IdentityNamespace: value.IdentityNamespace,
		ExternalID: value.ExternalID, DisplayName: value.DisplayName, HomepageURL: homepage,
	}
}

func citationPartyDTOs(values []CitationPartyReadDTO, expectedRole string) []CitationPartyDTO {
	result := make([]CitationPartyDTO, 0, len(values))
	for index := range values {
		if value := citationPartyDTO(&values[index], expectedRole); value != nil {
			result = append(result, *value)
		}
	}
	return result
}

func validCitationPartyKind(value string) bool {
	return value == "organization" || value == "person" || value == "account"
}

func validCitationPartyNamespace(value string) bool {
	if value == "" || len(value) > 64 || value != strings.ToLower(value) || value != strings.TrimSpace(value) {
		return false
	}
	for index, character := range value {
		alphanumeric := character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if alphanumeric || index > 0 && (character == '-' || character == '_' || character == '.' || character == ':') {
			continue
		}
		return false
	}
	return true
}

func validCitationPartyText(value string, maximumBytes int) bool {
	if value == "" || len(value) > maximumBytes || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func citationReadAvailability(read CitationReadDTO) (CitationAvailability, CitationUnavailableReason) {
	if read.ObservationState == "policy_blocked" {
		return CitationPolicyBlocked, CitationReasonPolicyBlocked
	}
	if read.DocumentState != "active" || (read.ObservationState != "active" && read.ObservationState != "corrected") {
		return CitationTombstoned, CitationReasonSourceUnavailable
	}
	switch read.DocumentLifecycleState {
	case DocumentPolicyBlocked:
		return CitationPolicyBlocked, CitationReasonPolicyBlocked
	case DocumentRetentionBlocked, DocumentTombstoned:
		return CitationTombstoned, CitationReasonRetentionUnavailable
	case DocumentQuarantined:
		return CitationQuarantined, CitationReasonIntegrityFailed
	}
	if !validLowerHexSHA256(read.ContentSHA256) || !validApplicationBodyOrigin(read.BodyOrigin) || !validApplicationBodyCompleteness(read.Completeness) || read.CapturedAt.IsZero() {
		return CitationQuarantined, CitationReasonIntegrityFailed
	}
	if read.Completeness == BodyCompletenessMetadataOnly {
		return CitationMetadataOnly, CitationReasonNoCitableBody
	}
	switch read.DocumentLifecycleState {
	case DocumentDerivedFailed:
		return CitationTemporarilyUnavailable, CitationReasonDocumentNotReadable
	case DocumentReadable:
		// Continue with current authorization and artifact checks.
	default:
		return CitationTemporarilyUnavailable, CitationReasonDocumentNotReadable
	}
	if !read.DisplayPrivateAllowed {
		return CitationPolicyBlocked, CitationReasonPermissionDenied
	}
	artifact := read.Artifact
	if artifact == nil {
		return CitationTemporarilyUnavailable, CitationReasonArtifactMissing
	}
	// Authorization and retention take precedence over artifact diagnostics so
	// an unauthorized caller cannot infer integrity details of inaccessible
	// bytes from the status classification.
	if !artifact.StoreDerivedAllowed {
		return CitationPolicyBlocked, CitationReasonPolicyBlocked
	}
	if !artifact.RetainAllowed || artifact.CurrentRetentionDays == nil || *artifact.CurrentRetentionDays <= 0 ||
		artifact.RetentionUntil.IsZero() || !artifact.RetentionUntil.After(read.RightsEvaluatedAt) ||
		artifact.RetentionUntil.After(read.CapturedAt.Add(time.Duration(*artifact.CurrentRetentionDays)*24*time.Hour)) {
		return CitationTombstoned, CitationReasonRetentionUnavailable
	}
	switch artifact.LifecycleState {
	case "retention_blocked", "tombstoned":
		return CitationTombstoned, CitationReasonRetentionUnavailable
	case "derive_failed", "quarantined":
		return CitationQuarantined, CitationReasonIntegrityFailed
	case "derived_available":
		// Continue with immutable manifest checks.
	default:
		return CitationTemporarilyUnavailable, CitationReasonDocumentNotReadable
	}
	if artifact.ArtifactType != "markdown" || artifact.MIMEType != "text/markdown; charset=utf-8" ||
		!validLowerHexSHA256(artifact.TransformerProfileSHA256) || !validLowerHexSHA256(artifact.SHA256) ||
		artifact.SizeBytes <= 0 || artifact.SizeBytes > citationDocumentMaximumBytes || !artifact.Active ||
		artifact.AvailableAt == nil || artifact.AvailableAt.IsZero() || artifact.FailureCode != nil {
		return CitationQuarantined, CitationReasonIntegrityFailed
	}
	if !validCitationArtifactAnchorMap(read.ContentSHA256, artifact) {
		return CitationQuarantined, CitationReasonIntegrityFailed
	}
	return citationArchiveAvailability(read), ""
}

func validCitationArtifactAnchorMap(contentSHA256 string, artifact *CitationArtifactReadDTO) bool {
	if artifact == nil || artifact.AnchorMap == nil || artifact.AnchorMap.PlaintextSHA256 != contentSHA256 ||
		artifact.AnchorMap.MarkdownSHA256 != artifact.SHA256 {
		return false
	}
	blocks := make([]DocumentAnchorBlockDTO, len(artifact.AnchorMap.Blocks))
	for index, block := range artifact.AnchorMap.Blocks {
		if block.MarkdownUTF8ByteEnd > artifact.SizeBytes {
			return false
		}
		blocks[index] = DocumentAnchorBlockDTO{
			Ordinal: block.Ordinal, PlaintextUTF8ByteStart: block.PlaintextUTF8ByteStart, PlaintextUTF8ByteEnd: block.PlaintextUTF8ByteEnd,
			MarkdownUTF8ByteStart: block.MarkdownUTF8ByteStart, MarkdownUTF8ByteEnd: block.MarkdownUTF8ByteEnd,
			MarkdownAnchor: block.MarkdownAnchor,
		}
	}
	return ValidatePersistedDocumentAnchorMap(&DerivedArtifactAnchorMapDTO{
		NormalizationVersion: artifact.AnchorMap.NormalizationVersion, AnchorMapProfileVersion: artifact.AnchorMap.AnchorMapProfileVersion,
		PlaintextSHA256: artifact.AnchorMap.PlaintextSHA256, MarkdownSHA256: artifact.AnchorMap.MarkdownSHA256,
		AnchorMapSHA256: artifact.AnchorMap.AnchorMapSHA256,
	}, blocks) == nil
}

func citationArtifactAnchorMapDTO(value *CitationArtifactAnchorMapReadDTO) *CitationArtifactAnchorMapDTO {
	if value == nil {
		return nil
	}
	result := &CitationArtifactAnchorMapDTO{
		NormalizationVersion: value.NormalizationVersion, AnchorMapProfileVersion: value.AnchorMapProfileVersion,
		AnchorMapSHA256: value.AnchorMapSHA256, Blocks: make([]CitationArtifactAnchorBlockDTO, len(value.Blocks)),
	}
	for index, block := range value.Blocks {
		result.Blocks[index] = CitationArtifactAnchorBlockDTO{Ordinal: block.Ordinal, MarkdownAnchor: block.MarkdownAnchor}
	}
	return result
}

func citationArchiveAvailability(read CitationReadDTO) CitationAvailability {
	if read.BodyOrigin == BodyOriginFeedSummary || read.BodyOrigin == BodyOriginSearchSnippet ||
		read.Completeness == BodyCompletenessSummary || read.Completeness == BodyCompletenessSnippet {
		return CitationSummaryOnly
	}
	if read.Completeness == BodyCompletenessFull {
		return CitationFullArchive
	}
	return CitationPartialArchive
}

func citationArchiveAvailable(availability CitationAvailability) bool {
	return availability == CitationFullArchive || availability == CitationPartialArchive || availability == CitationSummaryOnly
}

type DocumentReadFailureKind string

const (
	DocumentReadFailureNotReadable DocumentReadFailureKind = "not_readable"
	DocumentReadFailurePolicy      DocumentReadFailureKind = "policy"
	DocumentReadFailureRetention   DocumentReadFailureKind = "retention"
	DocumentReadFailureIntegrity   DocumentReadFailureKind = "integrity"
	DocumentReadFailurePermission  DocumentReadFailureKind = "permission"
	DocumentReadFailureMissing     DocumentReadFailureKind = "missing"
	DocumentReadFailureUnavailable DocumentReadFailureKind = "unavailable"
)

type DocumentReadError struct {
	Kind  DocumentReadFailureKind
	cause error
}

func (failure *DocumentReadError) Error() string {
	if failure == nil {
		return "document read failed"
	}
	return "document read " + string(failure.Kind)
}

func (failure *DocumentReadError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func newDocumentReadError(kind DocumentReadFailureKind, cause error) *DocumentReadError {
	return &DocumentReadError{Kind: kind, cause: cause}
}

func documentReadFailureForCitation(reason CitationUnavailableReason) DocumentReadFailureKind {
	switch reason {
	case CitationReasonPolicyBlocked:
		return DocumentReadFailurePolicy
	case CitationReasonRetentionUnavailable:
		return DocumentReadFailureRetention
	case CitationReasonIntegrityFailed:
		return DocumentReadFailureIntegrity
	case CitationReasonPermissionDenied:
		return DocumentReadFailurePermission
	case CitationReasonArtifactMissing:
		return DocumentReadFailureMissing
	default:
		return DocumentReadFailureNotReadable
	}
}

func validCitationIfNoneMatch(value string) bool {
	if value == "" {
		return true
	}
	return len(value) == 66 && value[0] == '"' && value[len(value)-1] == '"' && validLowerHexSHA256(value[1:len(value)-1])
}

func safeCitationURL(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len(trimmed) > 2048 || strings.Contains(trimmed, "#") {
		return nil
	}
	for _, character := range trimmed {
		if unicode.IsControl(character) {
			return nil
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.Hostname() == "" || parsed.Fragment != "" ||
		(!strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "http")) {
		return nil
	}
	result := parsed.String()
	return &result
}

func safeCitationText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func cloneCitationTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}
