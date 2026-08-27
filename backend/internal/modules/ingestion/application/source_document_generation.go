package application

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"golang.org/x/text/unicode/norm"
)

const (
	MaximumSelectedSourceEvidenceBytes = 4 << 20
	MaximumCanonicalSourceBodyBytes    = 4 << 20
	MaximumMarkdownProjectionBytes     = 4 << 20

	// CanonicalDocumentSearchNormalizationProfileVersion freezes the exact
	// memory-only plaintext identity consumed by lexical recall. The current
	// profile performs no additional rewrite after the extractor's UTF-8, LF,
	// trim and NFC canonicalization.
	CanonicalDocumentSearchNormalizationProfileVersion = "canonical-nfc-plaintext-structure-v1"
	CanonicalContentFingerprintProfileVersion          = "content-fingerprint-v1"
	CanonicalContentFamilyDecisionProfileVersion       = "content-family-decision-v1"
)

type SourceEvidenceQuery struct {
	EvidenceReferenceID int64
}

// SelectedSourceEvidenceDTO is Ingestion's own read projection. A future
// bootstrap adapter maps Source Application output into this type; Source
// Domain and persistence records never cross this boundary.
type SelectedSourceEvidenceDTO struct {
	EvidenceReferenceID int64
	SourceObservationID int64
	EvidenceSnapshotID  int64
	SourceConnectionID  int64

	ExternalWorkID            string
	UpstreamIdentity          string
	SourceCode                string
	ContentType               string
	Title                     string
	Language                  string
	Author                    string
	SourceRecordURL           string
	CanonicalURL              string
	DiscussionURL             string
	BodyOrigin                string
	Completeness              string
	PublishedAt               *time.Time
	PublishedUTCOffsetMinutes *int
	DiscoveredAt              time.Time
	CapturedAt                time.Time

	SelectedPayload       []byte
	SelectedPayloadSHA256 string
	PayloadMIMEType       string
	SelectorVersion       string
}

// SourceEvidenceReader returns only bytes already captured and reverified by
// Source. Its implementation must not perform a canonical-page fetch.
type SourceEvidenceReader interface {
	ReadSelectedSourceEvidence(context.Context, SourceEvidenceQuery) (SelectedSourceEvidenceDTO, error)
}

type ExtractSelectedSourceBodyCommand struct {
	Evidence SelectedSourceEvidenceDTO
}

// ExtractSelectedSourceBodyResult contains bounded canonical facts only. It
// cannot expose the selected XML bytes or the captured raw HTML field.
type ExtractSelectedSourceBodyResult struct {
	BodyOrigin                        string
	Completeness                      string
	Plaintext                         string
	Markdown                          string
	Language                          string
	ExtractorVersion                  string
	ExtractorProfileVersion           string
	ExtractorProfileSHA256            string
	PlaintextTransformerProfileSHA256 string
	MarkdownTransformerProfileSHA256  string
	PlaintextSHA256                   string
	MarkdownSHA256                    string
	TextNormalizationVersion          string
	AnchorMapProfileVersion           string
	AnchorMapSHA256                   string
	AnchorBlocks                      []DocumentAnchorBlockDTO
	Truncated                         bool
	QualityScore                      *float64
	QualityWarnings                   []string
}

type SelectedSourceBodyExtractor interface {
	Extract(context.Context, ExtractSelectedSourceBodyCommand) (ExtractSelectedSourceBodyResult, error)
}

type DocumentObservationPersister interface {
	PersistDocumentObservation(context.Context, PersistDocumentObservationCommand) (PersistDocumentVersionResult, error)
}

type DocumentProjectionAuthorizationQuery struct {
	SourceConnectionID int64
	DocumentVersionID  int64
	ContentSHA256      string
	DecisionAt         time.Time
}

// DocumentProjectionAuthorizationDTO represents three independent current
// decisions scoped to subject_type=document_version and the exact decimal
// document-version ID. StoreDerived and Retain must be explicit allows;
// DisplayPrivate is optional and is never inferred from raw-evidence rights.
type DocumentProjectionAuthorizationDTO struct {
	SourceConnectionID             int64
	DocumentVersionID              int64
	ContentSHA256                  string
	DecisionAt                     time.Time
	StoreDerivedRightsDecisionID   int64
	RetainRightsDecisionID         int64
	DisplayPrivateRightsDecisionID *int64
	EmbedLocalRightsDecisionID     *int64
}

// DocumentProjectionAuthorizationReader must fail closed unless all returned
// IDs are current exact-action allows for the query identity and digest.
type DocumentProjectionAuthorizationReader interface {
	ReadDocumentProjectionAuthorization(context.Context, DocumentProjectionAuthorizationQuery) (DocumentProjectionAuthorizationDTO, error)
}

type DocumentArtifactProjector interface {
	Project(context.Context, ProjectDocumentCommand) (ProjectDocumentResult, error)
}

type DocumentSearchProjectionPersister interface {
	PersistSearchProjection(context.Context, PersistDocumentSearchProjectionCommand) (DocumentSearchProjectionResult, error)
}

type DocumentContentFamilyAssigner interface {
	Assign(context.Context, AssignDocumentContentFamilyCommand) (AssignDocumentContentFamilyResult, error)
}

type GenerateSourceDocumentCommand struct {
	EvidenceReferenceID int64
}

// SourceDocumentAvailability reports which exact receipt this invocation has
// already verified. Readers still re-evaluate current rights when serving or
// recalling an artifact; a later Markdown failure does not erase a durable,
// independently verified search receipt.
type SourceDocumentAvailability string

const (
	SourceDocumentUnavailable   SourceDocumentAvailability = "unavailable"
	SourceDocumentAvailable     SourceDocumentAvailability = "available"
	SourceDocumentNotApplicable SourceDocumentAvailability = "not_applicable"
)

type GenerateSourceDocumentResult struct {
	PlaintextAvailability              SourceDocumentAvailability
	MarkdownAvailability               SourceDocumentAvailability
	SearchAvailability                 SourceDocumentAvailability
	EmbeddingAvailability              SourceDocumentAvailability
	EmbeddingUnavailableReason         string
	ContentFamilyAvailability          SourceDocumentAvailability
	DocumentID                         int64
	DocumentVersionID                  int64
	LastVerifiedDocumentVersion        int64
	LastVerifiedDocumentLifecycleState string
	DocumentCreated                    bool
	DocumentVersionCreated             bool
	ContentSHA256                      string
	PlaintextArtifact                  *DerivedArtifactDTO
	MarkdownArtifact                   *DerivedArtifactDTO
	SearchProjection                   *DocumentSearchProjectionResult
	EmbeddingReceipt                   *DocumentEmbeddingReceiptResult
	ContentFamilyDecision              *ContentFamilyDecisionDTO
}

type SourceDocumentGenerationDependencies struct {
	Evidence                  SourceEvidenceReader
	Extractor                 SelectedSourceBodyExtractor
	DocumentVersions          DocumentObservationPersister
	Authorizations            DocumentProjectionAuthorizationReader
	Projections               DocumentArtifactProjector
	SearchProjections         DocumentSearchProjectionPersister
	ContentFamilies           DocumentContentFamilyAssigner
	StructureExtractor        DocumentStructureExtractor
	DocumentEmbeddings        DocumentEmbeddingProducer
	PublishedMatchEvaluations PublishedDocumentMatchEvaluationScheduler
	Now                       func() time.Time
}

type SourceDocumentGenerationService struct {
	evidence                  SourceEvidenceReader
	extractor                 SelectedSourceBodyExtractor
	documentVersions          DocumentObservationPersister
	authorizations            DocumentProjectionAuthorizationReader
	projections               DocumentArtifactProjector
	searchProjections         DocumentSearchProjectionPersister
	contentFamilies           DocumentContentFamilyAssigner
	structureExtractor        DocumentStructureExtractor
	documentEmbeddings        DocumentEmbeddingProducer
	publishedMatchEvaluations PublishedDocumentMatchEvaluationScheduler
	now                       func() time.Time
}

func NewSourceDocumentGenerationService(dependencies SourceDocumentGenerationDependencies) (*SourceDocumentGenerationService, error) {
	if dependencies.Evidence == nil || dependencies.Extractor == nil || dependencies.DocumentVersions == nil ||
		dependencies.Authorizations == nil || dependencies.Projections == nil || dependencies.SearchProjections == nil || dependencies.StructureExtractor == nil ||
		dependencies.ContentFamilies == nil || dependencies.DocumentEmbeddings == nil || dependencies.PublishedMatchEvaluations == nil || dependencies.Now == nil {
		return nil, errors.New("source evidence, body extractor, document persistence, projection authorization, artifact writer, structure extractor, search projection writer, document embedding producer, published match scheduler, and clock are required")
	}
	return &SourceDocumentGenerationService{
		evidence: dependencies.Evidence, extractor: dependencies.Extractor, documentVersions: dependencies.DocumentVersions,
		authorizations: dependencies.Authorizations, projections: dependencies.Projections,
		searchProjections: dependencies.SearchProjections, structureExtractor: dependencies.StructureExtractor, documentEmbeddings: dependencies.DocumentEmbeddings,
		contentFamilies:           dependencies.ContentFamilies,
		publishedMatchEvaluations: dependencies.PublishedMatchEvaluations, now: dependencies.Now,
	}, nil
}

func (service *SourceDocumentGenerationService) Generate(ctx context.Context, command GenerateSourceDocumentCommand) (GenerateSourceDocumentResult, error) {
	if service == nil || service.evidence == nil || service.extractor == nil || service.documentVersions == nil ||
		service.authorizations == nil || service.projections == nil || service.searchProjections == nil || service.structureExtractor == nil || service.documentEmbeddings == nil ||
		service.contentFamilies == nil || service.publishedMatchEvaluations == nil || service.now == nil || command.EvidenceReferenceID <= 0 {
		return GenerateSourceDocumentResult{}, fmt.Errorf("%w: invalid source document generation input", sharedrepository.ErrInvalidInput)
	}
	evidence, err := service.evidence.ReadSelectedSourceEvidence(ctx, SourceEvidenceQuery{EvidenceReferenceID: command.EvidenceReferenceID})
	if err != nil {
		return GenerateSourceDocumentResult{}, fmt.Errorf("read selected source evidence: %w", err)
	}
	if err := validateSelectedSourceEvidence(evidence, command.EvidenceReferenceID); err != nil {
		return GenerateSourceDocumentResult{}, fmt.Errorf("%w: selected source evidence changed", sharedrepository.ErrConflict)
	}
	extracted, err := service.extractor.Extract(ctx, ExtractSelectedSourceBodyCommand{Evidence: cloneSelectedSourceEvidence(evidence)})
	if err != nil {
		return GenerateSourceDocumentResult{}, fmt.Errorf("extract selected source body: %w", err)
	}
	if err := validateExtractedSourceBody(extracted, evidence); err != nil {
		return GenerateSourceDocumentResult{}, fmt.Errorf("%w: extracted source body facts changed", sharedrepository.ErrConflict)
	}

	persisted, err := service.documentVersions.PersistDocumentObservation(ctx, PersistDocumentObservationCommand{
		Observation: DocumentObservationDTO{
			ID: evidence.SourceObservationID, SourceConnectionID: evidence.SourceConnectionID, ExternalWorkID: evidence.ExternalWorkID,
			BodyOrigin: extracted.BodyOrigin, Completeness: extracted.Completeness, Body: extracted.Plaintext,
			Language: extracted.Language, CapturedAt: evidence.CapturedAt,
		},
		ExtractorVersion: extracted.ExtractorVersion, ExtractorProfileVersion: extracted.ExtractorProfileVersion,
		ExtractorProfileSHA256: extracted.ExtractorProfileSHA256, Truncated: extracted.Truncated,
		QualityScore: cloneDocumentQualityScore(extracted.QualityScore), QualityWarnings: append([]string(nil), extracted.QualityWarnings...),
	})
	if err != nil {
		return GenerateSourceDocumentResult{}, fmt.Errorf("persist immutable document version: %w", err)
	}
	if err := validateGeneratedDocumentVersion(persisted, evidence, extracted); err != nil {
		return GenerateSourceDocumentResult{}, fmt.Errorf("%w: persisted document version changed", sharedrepository.ErrConflict)
	}
	baseResult := generatedSourceDocumentResult(persisted)
	if extracted.Completeness == BodyCompletenessMetadataOnly {
		baseResult.PlaintextAvailability = SourceDocumentNotApplicable
		baseResult.MarkdownAvailability = SourceDocumentNotApplicable
		baseResult.SearchAvailability = SourceDocumentNotApplicable
		baseResult.EmbeddingAvailability = SourceDocumentNotApplicable
		baseResult.ContentFamilyAvailability = SourceDocumentNotApplicable
		// The immutable version uses an internal empty-content digest for its
		// identity constraint, but metadata-only output must not project that
		// implementation detail as evidence that body bytes exist.
		baseResult.ContentSHA256 = ""
		return baseResult, nil
	}

	decisionAt := service.now().UTC()
	if decisionAt.IsZero() {
		return baseResult, fmt.Errorf("%w: projection decision time is invalid", sharedrepository.ErrInvalidInput)
	}
	authorizationQuery := DocumentProjectionAuthorizationQuery{
		SourceConnectionID: evidence.SourceConnectionID, DocumentVersionID: persisted.DocumentVersion.ID,
		ContentSHA256: persisted.DocumentVersion.ContentSHA256, DecisionAt: decisionAt,
	}
	if err := ValidateDocumentProjectionAuthorizationQuery(authorizationQuery); err != nil {
		return baseResult, fmt.Errorf("%w: document projection authorization query is invalid", sharedrepository.ErrInvalidInput)
	}
	authorization, err := service.authorizations.ReadDocumentProjectionAuthorization(ctx, authorizationQuery)
	if err != nil {
		return baseResult, fmt.Errorf("resolve exact document projection authorization: %w", err)
	}
	if err := ValidateDocumentProjectionAuthorizationDTO(authorization, authorizationQuery); err != nil {
		return baseResult, fmt.Errorf("%w: document projection authorization changed", sharedrepository.ErrConflict)
	}

	plaintextProjected, err := service.projections.Project(ctx, ProjectDocumentCommand{
		DocumentVersionID: persisted.DocumentVersion.ID, ExpectedDocumentVersion: persisted.DocumentVersion.Version,
		ArtifactType: DocumentProjectionPlaintext, TransformerProfileSHA256: extracted.PlaintextTransformerProfileSHA256,
		StoreDerivedRightsDecisionID: authorization.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       authorization.RetainRightsDecisionID,
		ProjectionBytes:              []byte(extracted.Plaintext),
	})
	if err != nil {
		return baseResult, fmt.Errorf("project authorized plaintext artifact: %w", err)
	}
	if err := validateGeneratedDocumentProjection(
		plaintextProjected, persisted, authorization, DocumentProjectionPlaintext,
		extracted.PlaintextTransformerProfileSHA256, []byte(extracted.Plaintext), persisted.DocumentVersion.Version, nil,
	); err != nil {
		return baseResult, fmt.Errorf("%w: plaintext projection receipt changed", sharedrepository.ErrConflict)
	}
	baseResult = withGeneratedArtifact(baseResult, plaintextProjected, DocumentProjectionPlaintext)
	structureCommand := ExtractDocumentStructureCommand{
		DocumentVersionID: persisted.DocumentVersion.ID,
		ContentSHA256:     persisted.DocumentVersion.ContentSHA256,
		Title:             evidence.Title,
		Plaintext:         extracted.Plaintext,
		Language:          extracted.Language,
	}
	if err := validateDocumentStructureCommand(structureCommand); err != nil {
		return baseResult, err
	}
	structure, err := service.structureExtractor.ExtractDocumentStructure(ctx, structureCommand)
	if err != nil {
		return baseResult, fmt.Errorf("extract local document structure: %w", err)
	}
	if err := validateDocumentStructureResult(structure, structureCommand); err != nil {
		return baseResult, err
	}

	searchProjected, err := service.searchProjections.PersistSearchProjection(ctx, PersistDocumentSearchProjectionCommand{
		DocumentVersionID: persisted.DocumentVersion.ID, DerivedArtifactID: plaintextProjected.Artifact.ID,
		StoreDerivedRightsDecisionID: authorization.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       authorization.RetainRightsDecisionID,
		NormalizationProfileVersion:  CanonicalDocumentSearchNormalizationProfileVersion,
		NormalizedTextSHA256:         extracted.PlaintextSHA256,
		Plaintext:                    extracted.Plaintext,
		EntityKeys:                   cloneDocumentStructureKeys(structure.EntityKeys),
		ActionKeys:                   cloneDocumentStructureKeys(structure.ActionKeys),
		LocationKeys:                 cloneDocumentStructureKeys(structure.LocationKeys),
		RegionKeys:                   cloneDocumentStructureKeys(structure.RegionKeys),
		IndexedAt:                    authorization.DecisionAt,
	})
	if err != nil {
		return baseResult, fmt.Errorf("persist exact document search projection: %w", err)
	}
	if err := validateGeneratedSearchProjection(searchProjected, persisted, plaintextProjected.Artifact, authorization, extracted); err != nil {
		return baseResult, fmt.Errorf("%w: document search projection receipt changed", sharedrepository.ErrConflict)
	}
	baseResult.SearchAvailability = SourceDocumentAvailable
	searchReceipt := searchProjected
	baseResult.SearchProjection = &searchReceipt

	familyAssignment, err := service.contentFamilies.Assign(ctx, AssignDocumentContentFamilyCommand{
		SourceConnectionID: evidence.SourceConnectionID, DocumentVersionID: persisted.DocumentVersion.ID,
		DerivedArtifactID:            plaintextProjected.Artifact.ID,
		StoreDerivedRightsDecisionID: authorization.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       authorization.RetainRightsDecisionID,
		RetentionUntil:               plaintextProjected.Artifact.RetentionUntil, DecisionAt: authorization.DecisionAt,
		CanonicalPlaintext: extracted.Plaintext, FingerprintProfile: CanonicalContentFingerprintProfileVersion,
		DecisionProfileVersion: CanonicalContentFamilyDecisionProfileVersion,
	})
	if err != nil {
		return baseResult, fmt.Errorf("assign exact document content family: %w", err)
	}
	if familyAssignment.Decision.DecisionID <= 0 || familyAssignment.Decision.DocumentVersionID != persisted.DocumentVersion.ID ||
		familyAssignment.Decision.FamilyID <= 0 || familyAssignment.Decision.RootDocumentVersionID <= 0 ||
		familyAssignment.Decision.DecisionProfileVersion != CanonicalContentFamilyDecisionProfileVersion {
		return baseResult, fmt.Errorf("%w: content family receipt changed", sharedrepository.ErrConflict)
	}
	baseResult.ContentFamilyAvailability = SourceDocumentAvailable
	familyReceipt := familyAssignment.Decision
	baseResult.ContentFamilyDecision = &familyReceipt

	if authorization.EmbedLocalRightsDecisionID == nil {
		baseResult.EmbeddingAvailability = SourceDocumentUnavailable
		baseResult.EmbeddingUnavailableReason = DocumentEmbeddingReasonPolicyUnavailable
	} else {
		embeddingCommand := ProduceDocumentEmbeddingCommand{
			DocumentVersionID: persisted.DocumentVersion.ID, SourceConnectionID: evidence.SourceConnectionID,
			EmbedLocalRightsDecisionID: *authorization.EmbedLocalRightsDecisionID,
			RetainRightsDecisionID:     authorization.RetainRightsDecisionID,
			NormalizedTextSHA256:       extracted.PlaintextSHA256, Plaintext: extracted.Plaintext,
		}
		embedding, err := service.documentEmbeddings.ProduceDocumentEmbedding(ctx, embeddingCommand)
		if err != nil {
			return baseResult, fmt.Errorf("produce exact document embedding: %w", err)
		}
		if err := validateProducedDocumentEmbedding(embeddingCommand, embedding); err != nil {
			return baseResult, fmt.Errorf("%w: document embedding receipt changed", sharedrepository.ErrConflict)
		}
		baseResult.EmbeddingAvailability = embedding.Availability
		baseResult.EmbeddingUnavailableReason = embedding.UnavailableReason
		if embedding.Receipt != nil {
			receipt := *embedding.Receipt
			baseResult.EmbeddingReceipt = &receipt
		}
	}

	scheduleCommand := SchedulePublishedDocumentMatchEvaluationCommand{DocumentVersionID: persisted.DocumentVersion.ID}
	scheduled, err := service.publishedMatchEvaluations.SchedulePublishedDocumentMatchEvaluation(ctx, scheduleCommand)
	if err != nil {
		return baseResult, fmt.Errorf("schedule published document match evaluation: %w", err)
	}
	if err := validatePublishedDocumentMatchScheduleReceipt(scheduleCommand, scheduled); err != nil {
		return baseResult, fmt.Errorf("%w: published document match schedule receipt changed", sharedrepository.ErrConflict)
	}

	markdownProjected, err := service.projections.Project(ctx, ProjectDocumentCommand{
		DocumentVersionID: persisted.DocumentVersion.ID, ExpectedDocumentVersion: plaintextProjected.DocumentVersion.Version,
		ArtifactType: DocumentProjectionMarkdown, TransformerProfileSHA256: extracted.MarkdownTransformerProfileSHA256,
		StoreDerivedRightsDecisionID:   authorization.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:         authorization.RetainRightsDecisionID,
		DisplayPrivateRightsDecisionID: copyAuthorizationDecisionID(authorization.DisplayPrivateRightsDecisionID),
		ProjectionBytes:                []byte(extracted.Markdown),
		AnchorMap:                      projectDocumentAnchorMapFromExtraction(extracted),
	})
	if err != nil {
		return baseResult, fmt.Errorf("project authorized Markdown artifact: %w", err)
	}
	if err := validateGeneratedDocumentProjection(
		markdownProjected, persisted, authorization, DocumentProjectionMarkdown,
		extracted.MarkdownTransformerProfileSHA256, []byte(extracted.Markdown), plaintextProjected.DocumentVersion.Version,
		projectDocumentAnchorMapFromExtraction(extracted),
	); err != nil {
		return baseResult, fmt.Errorf("%w: Markdown projection receipt changed", sharedrepository.ErrConflict)
	}
	baseResult = withGeneratedArtifact(baseResult, markdownProjected, DocumentProjectionMarkdown)
	return baseResult, nil
}

func validateSelectedSourceEvidence(evidence SelectedSourceEvidenceDTO, requestedReferenceID int64) error {
	if evidence.EvidenceReferenceID != requestedReferenceID || evidence.SourceObservationID <= 0 || evidence.EvidenceSnapshotID <= 0 ||
		evidence.SourceConnectionID <= 0 || strings.TrimSpace(evidence.ExternalWorkID) == "" || !supportedSourceEvidenceCode(evidence.SourceCode) ||
		!validLowerHexSHA256(evidence.UpstreamIdentity) ||
		strings.TrimSpace(evidence.ContentType) == "" || !validApplicationBodyOrigin(evidence.BodyOrigin) ||
		!validApplicationBodyCompleteness(evidence.Completeness) || evidence.CapturedAt.IsZero() || len(evidence.SelectedPayload) == 0 ||
		len(evidence.SelectedPayload) > MaximumSelectedSourceEvidenceBytes || !utf8.Valid(evidence.SelectedPayload) ||
		!validLowerHexSHA256(evidence.SelectedPayloadSHA256) || strings.TrimSpace(evidence.PayloadMIMEType) == "" ||
		strings.TrimSpace(evidence.SelectorVersion) == "" {
		return errors.New("selected source evidence is invalid")
	}
	if evidence.BodyOrigin != BodyOriginFeedContent && evidence.BodyOrigin != BodyOriginFeedSummary &&
		evidence.BodyOrigin != BodyOriginPlatformPost && evidence.BodyOrigin != BodyOriginSearchSnippet && evidence.BodyOrigin != BodyOriginAPIContent {
		return errors.New("selected source evidence body origin is unsupported")
	}
	if evidence.Completeness != BodyCompletenessFull && evidence.Completeness != BodyCompletenessSummary && evidence.Completeness != BodyCompletenessSnippet &&
		evidence.Completeness != BodyCompletenessMetadataOnly {
		return errors.New("selected source evidence completeness is unsupported")
	}
	if evidence.PublishedAt == nil && evidence.PublishedUTCOffsetMinutes != nil {
		return errors.New("selected source evidence published UTC offset requires a published time")
	}
	if evidence.PublishedUTCOffsetMinutes != nil && (*evidence.PublishedUTCOffsetMinutes < -840 || *evidence.PublishedUTCOffsetMinutes > 840) {
		return errors.New("selected source evidence published UTC offset is invalid")
	}
	digest := sha256.Sum256(evidence.SelectedPayload)
	declared, err := hex.DecodeString(evidence.SelectedPayloadSHA256)
	if err != nil || len(declared) != sha256.Size || subtle.ConstantTimeCompare(digest[:], declared) != 1 {
		return errors.New("selected source evidence digest is invalid")
	}
	return nil
}

func supportedSourceEvidenceCode(value string) bool {
	switch value {
	case "rss", "x", "bilibili", "hacker_news", "weibo", "google_agent_search", "bing_grounding":
		return true
	default:
		return false
	}
}

func validateExtractedSourceBody(extracted ExtractSelectedSourceBodyResult, evidence SelectedSourceEvidenceDTO) error {
	if extracted.BodyOrigin != evidence.BodyOrigin || extracted.Completeness != evidence.Completeness || !validApplicationBodyOrigin(extracted.BodyOrigin) ||
		!validApplicationBodyCompleteness(extracted.Completeness) || len(extracted.Plaintext) > MaximumCanonicalSourceBodyBytes ||
		len(extracted.Markdown) > MaximumMarkdownProjectionBytes || !utf8.ValidString(extracted.Plaintext) || !utf8.ValidString(extracted.Markdown) ||
		strings.TrimSpace(extracted.ExtractorVersion) != extracted.ExtractorVersion || extracted.ExtractorVersion == "" ||
		strings.TrimSpace(extracted.ExtractorProfileVersion) != extracted.ExtractorProfileVersion || extracted.ExtractorProfileVersion == "" ||
		!validLowerHexSHA256(extracted.ExtractorProfileSHA256) ||
		!validLowerHexSHA256(extracted.PlaintextTransformerProfileSHA256) ||
		!validLowerHexSHA256(extracted.MarkdownTransformerProfileSHA256) {
		return errors.New("extracted source body is invalid")
	}
	canonicalPlaintext := norm.NFC.String(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(extracted.Plaintext, "\r\n", "\n"), "\r", "\n")))
	if canonicalPlaintext != extracted.Plaintext {
		return errors.New("extracted source plaintext is not canonical")
	}
	if extracted.Completeness == BodyCompletenessMetadataOnly {
		if extracted.Plaintext != "" || extracted.Markdown != "" || extracted.PlaintextSHA256 != "" || extracted.MarkdownSHA256 != "" ||
			extracted.TextNormalizationVersion != "" || extracted.AnchorMapProfileVersion != "" || extracted.AnchorMapSHA256 != "" || len(extracted.AnchorBlocks) != 0 {
			return errors.New("metadata-only extraction contains body bytes")
		}
		return nil
	}
	if extracted.Plaintext == "" || strings.TrimSpace(extracted.Markdown) == "" ||
		!validLowerHexSHA256(extracted.PlaintextSHA256) || strings.ContainsRune(extracted.Markdown, '\r') ||
		norm.NFC.String(extracted.Markdown) != extracted.Markdown {
		return errors.New("body extraction has no canonical projections")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(extracted.Plaintext)))
	if digest != extracted.PlaintextSHA256 {
		return errors.New("extracted source plaintext digest is invalid")
	}
	mapResult := MapDocumentTextResult{
		Plaintext: extracted.Plaintext, NormalizationVersion: extracted.TextNormalizationVersion,
		AnchorMapProfileVersion: extracted.AnchorMapProfileVersion, PlaintextSHA256: extracted.PlaintextSHA256,
		MarkdownSHA256: extracted.MarkdownSHA256, AnchorMapSHA256: extracted.AnchorMapSHA256,
		Blocks: append([]DocumentAnchorBlockDTO(nil), extracted.AnchorBlocks...),
	}
	if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: extracted.Markdown}, mapResult); err != nil {
		return errors.New("extracted source anchor map is invalid")
	}
	return nil
}

func validateGeneratedDocumentVersion(persisted PersistDocumentVersionResult, evidence SelectedSourceEvidenceDTO, extracted ExtractSelectedSourceBodyResult) error {
	document := persisted.Document
	version := persisted.DocumentVersion
	returnIfInvalid := document.ID <= 0 || document.Version <= 0 || document.SourceConnectionID != evidence.SourceConnectionID ||
		document.State != DocumentStateActive || version.ID <= 0 || version.Version <= 0 || version.DocumentID != document.ID ||
		version.SourceObservationID != evidence.SourceObservationID || version.BodyOrigin != extracted.BodyOrigin ||
		version.Completeness != extracted.Completeness || version.ContentSHA256 != extractedSourceContentSHA256(extracted) ||
		version.ExtractorVersion != extracted.ExtractorVersion || version.ExtractorProfileVersion != extracted.ExtractorProfileVersion ||
		version.ExtractorProfileSHA256 != extracted.ExtractorProfileSHA256 ||
		!validApplicationDocumentLifecycle(version.LifecycleState)
	if returnIfInvalid {
		return errors.New("persisted document version is inconsistent")
	}
	return nil
}

func ValidateDocumentProjectionAuthorizationQuery(query DocumentProjectionAuthorizationQuery) error {
	if query.SourceConnectionID <= 0 || query.DocumentVersionID <= 0 || !validLowerHexSHA256(query.ContentSHA256) || query.DecisionAt.IsZero() {
		return errors.New("document projection authorization query is invalid")
	}
	return nil
}

func ValidateDocumentProjectionAuthorizationDTO(authorization DocumentProjectionAuthorizationDTO, query DocumentProjectionAuthorizationQuery) error {
	if err := ValidateDocumentProjectionAuthorizationQuery(query); err != nil {
		return err
	}
	if authorization.SourceConnectionID != query.SourceConnectionID || authorization.DocumentVersionID != query.DocumentVersionID ||
		authorization.ContentSHA256 != query.ContentSHA256 || !authorization.DecisionAt.Equal(query.DecisionAt) ||
		authorization.StoreDerivedRightsDecisionID <= 0 || authorization.RetainRightsDecisionID <= 0 {
		return errors.New("document projection authorization is inconsistent")
	}
	if authorization.DisplayPrivateRightsDecisionID != nil && *authorization.DisplayPrivateRightsDecisionID <= 0 {
		return errors.New("document display authorization is invalid")
	}
	if authorization.EmbedLocalRightsDecisionID != nil && *authorization.EmbedLocalRightsDecisionID <= 0 {
		return errors.New("document embedding authorization is invalid")
	}
	return nil
}

func generatedSourceDocumentResult(persisted PersistDocumentVersionResult) GenerateSourceDocumentResult {
	return GenerateSourceDocumentResult{
		PlaintextAvailability: SourceDocumentUnavailable, MarkdownAvailability: SourceDocumentUnavailable,
		SearchAvailability: SourceDocumentUnavailable, EmbeddingAvailability: SourceDocumentUnavailable,
		ContentFamilyAvailability: SourceDocumentUnavailable,
		DocumentID:                persisted.Document.ID, DocumentVersionID: persisted.DocumentVersion.ID,
		LastVerifiedDocumentVersion:        persisted.DocumentVersion.Version,
		LastVerifiedDocumentLifecycleState: persisted.DocumentVersion.LifecycleState,
		DocumentCreated:                    persisted.DocumentCreated, DocumentVersionCreated: persisted.DocumentVersionCreated,
		ContentSHA256: persisted.DocumentVersion.ContentSHA256,
	}
}

func extractedSourceContentSHA256(extracted ExtractSelectedSourceBodyResult) string {
	if extracted.Completeness == BodyCompletenessMetadataOnly {
		return fmt.Sprintf("%x", sha256.Sum256(nil))
	}
	return extracted.PlaintextSHA256
}

func validateGeneratedDocumentProjection(
	projected ProjectDocumentResult,
	persisted PersistDocumentVersionResult,
	authorization DocumentProjectionAuthorizationDTO,
	artifactType string,
	transformerProfileSHA256 string,
	content []byte,
	minimumDocumentVersion int64,
	anchorMap *ProjectDocumentAnchorMapCommand,
) error {
	artifact, artifactErr := derivedArtifactDomainFromDTO(projected.Artifact)
	version, versionErr := documentVersionDomainFromDTO(projected.DocumentVersion)
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	if artifactErr != nil || versionErr != nil || artifact.ID <= 0 ||
		artifact.SourceConnectionID != authorization.SourceConnectionID ||
		artifact.DocumentVersionID != persisted.DocumentVersion.ID ||
		string(artifact.ArtifactType) != artifactType || artifact.TransformerProfileSHA256 != transformerProfileSHA256 ||
		artifact.SHA256 != digest || artifact.SizeBytes != int64(len(content)) ||
		artifact.StoreDerivedRightsDecisionID != authorization.StoreDerivedRightsDecisionID ||
		artifact.RetainRightsDecisionID != authorization.RetainRightsDecisionID ||
		string(artifact.LifecycleState) != DerivedArtifactAvailable || !artifact.Active ||
		version.ID != persisted.DocumentVersion.ID || version.DocumentID != persisted.Document.ID ||
		version.SourceObservationID != persisted.DocumentVersion.SourceObservationID ||
		version.ContentSHA256 != persisted.DocumentVersion.ContentSHA256 || version.Version < minimumDocumentVersion ||
		!validApplicationDocumentLifecycle(string(version.LifecycleState)) {
		return errors.New("document projection receipt is inconsistent")
	}
	if artifactType == DocumentProjectionMarkdown {
		if anchorMap == nil || artifact.AnchorMap == nil || artifact.AnchorMap.NormalizationVersion != anchorMap.NormalizationVersion ||
			artifact.AnchorMap.AnchorMapProfileVersion != anchorMap.AnchorMapProfileVersion ||
			artifact.AnchorMap.PlaintextSHA256 != anchorMap.PlaintextSHA256 || artifact.AnchorMap.MarkdownSHA256 != anchorMap.MarkdownSHA256 ||
			artifact.AnchorMap.AnchorMapSHA256 != anchorMap.AnchorMapSHA256 {
			return errors.New("Markdown anchor map receipt is inconsistent")
		}
	} else if artifact.AnchorMap != nil {
		return errors.New("plaintext projection returned a Markdown anchor map")
	}
	return nil
}

func projectDocumentAnchorMapFromExtraction(extracted ExtractSelectedSourceBodyResult) *ProjectDocumentAnchorMapCommand {
	return &ProjectDocumentAnchorMapCommand{
		Plaintext: extracted.Plaintext, NormalizationVersion: extracted.TextNormalizationVersion,
		AnchorMapProfileVersion: extracted.AnchorMapProfileVersion, PlaintextSHA256: extracted.PlaintextSHA256,
		MarkdownSHA256: extracted.MarkdownSHA256, AnchorMapSHA256: extracted.AnchorMapSHA256,
		Blocks: cloneDocumentAnchorBlocks(extracted.AnchorBlocks),
	}
}

func validateGeneratedSearchProjection(
	projected DocumentSearchProjectionResult,
	persisted PersistDocumentVersionResult,
	plaintextArtifact DerivedArtifactDTO,
	authorization DocumentProjectionAuthorizationDTO,
	extracted ExtractSelectedSourceBodyResult,
) error {
	if projected.ProjectionID <= 0 || projected.DocumentVersionID != persisted.DocumentVersion.ID ||
		projected.SourceConnectionID != persisted.Document.SourceConnectionID ||
		projected.SourceConnectionID != authorization.SourceConnectionID ||
		projected.DerivedArtifactID != plaintextArtifact.ID ||
		projected.StoreDerivedRightsDecisionID != authorization.StoreDerivedRightsDecisionID ||
		projected.RetainRightsDecisionID != authorization.RetainRightsDecisionID ||
		projected.NormalizationProfileVersion != CanonicalDocumentSearchNormalizationProfileVersion ||
		projected.NormalizedTextSHA256 != extracted.PlaintextSHA256 ||
		projected.IndexedAt.IsZero() || projected.IndexedAt.After(authorization.DecisionAt) ||
		(projected.Created && !projected.IndexedAt.Equal(authorization.DecisionAt)) ||
		!projected.RetentionUntil.Equal(plaintextArtifact.RetentionUntil) ||
		!projected.RetentionUntil.After(authorization.DecisionAt) ||
		projected.LifecycleState != RecallAssetLifecycleActive {
		return errors.New("document search projection receipt is inconsistent")
	}
	return nil
}

func withGeneratedArtifact(result GenerateSourceDocumentResult, projected ProjectDocumentResult, artifactType string) GenerateSourceDocumentResult {
	result.LastVerifiedDocumentVersion = projected.DocumentVersion.Version
	result.LastVerifiedDocumentLifecycleState = projected.DocumentVersion.LifecycleState
	artifact := projected.Artifact
	switch artifactType {
	case DocumentProjectionPlaintext:
		result.PlaintextAvailability = SourceDocumentAvailable
		result.PlaintextArtifact = &artifact
	case DocumentProjectionMarkdown:
		result.MarkdownAvailability = SourceDocumentAvailable
		result.MarkdownArtifact = &artifact
	}
	return result
}

func cloneSelectedSourceEvidence(evidence SelectedSourceEvidenceDTO) SelectedSourceEvidenceDTO {
	copy := evidence
	copy.SelectedPayload = append([]byte(nil), evidence.SelectedPayload...)
	if evidence.PublishedAt != nil {
		publishedAt := evidence.PublishedAt.UTC()
		copy.PublishedAt = &publishedAt
	}
	if evidence.PublishedUTCOffsetMinutes != nil {
		offset := *evidence.PublishedUTCOffsetMinutes
		copy.PublishedUTCOffsetMinutes = &offset
	}
	return copy
}

func copyAuthorizationDecisionID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
