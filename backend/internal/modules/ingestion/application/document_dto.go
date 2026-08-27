package application

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

const (
	BodyOriginAPIContent                  = "api_content"
	BodyOriginFeedContent                 = "feed_content"
	BodyOriginFeedSummary                 = "feed_summary"
	BodyOriginStructuredArticleBody       = "structured_article_body"
	BodyOriginAuthorizedPayloadExtraction = "authorized_payload_extraction"
	BodyOriginPlatformPost                = "platform_post"
	BodyOriginSearchSnippet               = "search_snippet"

	BodyCompletenessFull         = "full"
	BodyCompletenessPartial      = "partial"
	BodyCompletenessSummary      = "summary"
	BodyCompletenessSnippet      = "snippet"
	BodyCompletenessMetadataOnly = "metadata_only"
	BodyCompletenessUnknown      = "unknown"

	DocumentStateActive     = "active"
	DocumentStateWithdrawn  = "withdrawn"
	DocumentStateTombstoned = "tombstoned"

	DocumentPolicyPending    = "policy_pending"
	DocumentPolicyBlocked    = "policy_blocked"
	DocumentRawPending       = "raw_pending"
	DocumentRawAvailable     = "raw_available"
	DocumentRawFailed        = "raw_failed"
	DocumentDerivedPending   = "derive_pending"
	DocumentDerivedAvailable = "derived_available"
	DocumentDerivedFailed    = "derive_failed"
	DocumentReadable         = "readable"
	DocumentRetentionBlocked = "retention_blocked"
	DocumentQuarantined      = "quarantined"
	DocumentTombstoned       = "tombstoned"

	DerivedArtifactMarkdown  = "markdown"
	DerivedArtifactPlaintext = "plaintext"

	DerivedArtifactPending          = "derive_pending"
	DerivedArtifactAvailable        = "derived_available"
	DerivedArtifactFailed           = "derive_failed"
	DerivedArtifactRetentionBlocked = "retention_blocked"
	DerivedArtifactQuarantined      = "quarantined"
	DerivedArtifactTombstoned       = "tombstoned"
)

// DocumentIdentityDTO is the persistence-facing identity of a stable
// Document container. It contains no body and no Domain value object.
type DocumentIdentityDTO struct {
	SourceConnectionID int64
	DocumentKey        string
	ExternalWorkID     *string
	CanonicalURL       string
	ContentSHA256      string
}

// DocumentDTO is the Application projection of one stable Document entity.
// Pointer and time fields are copied at every Domain boundary.
type DocumentDTO struct {
	ID                 int64
	Version            int64
	SourceConnectionID int64
	DocumentKey        string
	ExternalWorkID     *string
	CurrentVersionID   *int64
	State              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// DocumentVersionDTO contains immutable normalized facts and mutable
// lifecycle metadata. The canonical body is intentionally absent.
type DocumentVersionDTO struct {
	ID                             int64
	Version                        int64
	DocumentID                     int64
	SourceObservationID            int64
	RevisionNo                     int64
	VersionKey                     string
	BodyOrigin                     string
	Completeness                   string
	WordCount                      int
	Language                       string
	Truncated                      bool
	QualityScore                   *float64
	QualityWarnings                []string
	ContentSHA256                  string
	ExtractorVersion               string
	ExtractorProfileVersion        string
	ExtractorProfileSHA256         string
	DisplayPrivateRightsDecisionID *int64
	LifecycleState                 string
	CapturedAt                     time.Time
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

// DerivedArtifactDTO is the Application projection of one immutable Vault
// manifest. It never contains projection bytes or an absolute provider path.
type DerivedArtifactDTO struct {
	ID                           int64
	SourceConnectionID           int64
	DocumentVersionID            int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	ArtifactType                 string
	TransformerProfileSHA256     string
	MIMEType                     string
	SHA256                       string
	SizeBytes                    int64
	LifecycleState               string
	Active                       bool
	FailureCode                  string
	AvailableAt                  *time.Time
	RetentionUntil               time.Time
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	AnchorMap                    *DerivedArtifactAnchorMapDTO
}

type DerivedArtifactAnchorMapDTO struct {
	NormalizationVersion    string
	AnchorMapProfileVersion string
	PlaintextSHA256         string
	MarkdownSHA256          string
	AnchorMapSHA256         string
}

func ValidateDocumentDTO(value DocumentDTO) error {
	_, err := documentDomainFromDTO(value)
	return err
}

func ValidateDocumentIdentityDTO(value DocumentIdentityDTO) error {
	if _, err := documentIdentityDomainFromDTO(value); err != nil {
		return err
	}
	if value.ContentSHA256 != "" && !validLowerHexSHA256(value.ContentSHA256) {
		return fmt.Errorf("document content identity is invalid")
	}
	if value.CanonicalURL != "" {
		parsed, err := url.Parse(value.CanonicalURL)
		if err != nil || parsed == nil || parsed.User != nil || parsed.Fragment != "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
			value.CanonicalURL != strings.TrimSpace(value.CanonicalURL) || len(value.CanonicalURL) > 2048 {
			return fmt.Errorf("document canonical URL identity is invalid")
		}
	}
	return nil
}

func ValidateDocumentVersionDTO(value DocumentVersionDTO) error {
	_, err := documentVersionDomainFromDTO(value)
	return err
}

func ValidateDerivedArtifactDTO(value DerivedArtifactDTO) error {
	_, err := derivedArtifactDomainFromDTO(value)
	return err
}

func ValidateDocumentBodyClassification(bodyOrigin, completeness string) error {
	if !validApplicationBodyOrigin(bodyOrigin) || !validApplicationBodyCompleteness(completeness) {
		return fmt.Errorf("document body classification is invalid")
	}
	return nil
}

func documentIdentityDomainFromDTO(value DocumentIdentityDTO) (ingestiondomain.DocumentIdentity, error) {
	identity := ingestiondomain.DocumentIdentity{
		SourceConnectionID: value.SourceConnectionID,
		DocumentKey:        value.DocumentKey,
		ExternalWorkID:     copyDocumentString(value.ExternalWorkID),
	}
	if err := identity.Validate(); err != nil {
		return ingestiondomain.DocumentIdentity{}, err
	}
	return identity, nil
}

func documentIdentityDTOFromDomain(value ingestiondomain.DocumentIdentity) DocumentIdentityDTO {
	return DocumentIdentityDTO{
		SourceConnectionID: value.SourceConnectionID,
		DocumentKey:        value.DocumentKey,
		ExternalWorkID:     copyDocumentString(value.ExternalWorkID),
	}
}

func documentDTOFromDomain(value ingestiondomain.Document) DocumentDTO {
	return DocumentDTO{
		ID: value.ID, Version: value.Version, SourceConnectionID: value.SourceConnectionID,
		DocumentKey: value.DocumentKey, ExternalWorkID: copyDocumentString(value.ExternalWorkID),
		CurrentVersionID: copyDocumentInt64(value.CurrentVersionID), State: string(value.State),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func documentDomainFromDTO(value DocumentDTO) (ingestiondomain.Document, error) {
	identity, err := documentIdentityDomainFromDTO(DocumentIdentityDTO{
		SourceConnectionID: value.SourceConnectionID,
		DocumentKey:        value.DocumentKey,
		ExternalWorkID:     value.ExternalWorkID,
	})
	if err != nil || value.ID <= 0 || value.Version <= 0 || !ingestiondomain.DocumentState(value.State).Valid() ||
		(value.CurrentVersionID != nil && *value.CurrentVersionID <= 0) || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() ||
		value.UpdatedAt.Before(value.CreatedAt) {
		return ingestiondomain.Document{}, fmt.Errorf("document DTO is invalid")
	}
	return ingestiondomain.Document{
		ID: value.ID, Version: value.Version, SourceConnectionID: identity.SourceConnectionID,
		DocumentKey: identity.DocumentKey, ExternalWorkID: copyDocumentString(identity.ExternalWorkID),
		CurrentVersionID: copyDocumentInt64(value.CurrentVersionID), State: ingestiondomain.DocumentState(value.State),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}, nil
}

func documentVersionDTOFromDomain(value ingestiondomain.DocumentVersion) DocumentVersionDTO {
	return DocumentVersionDTO{
		ID: value.ID, Version: value.Version, DocumentID: value.DocumentID,
		SourceObservationID: value.SourceObservationID, RevisionNo: value.RevisionNo,
		VersionKey: value.VersionKey, BodyOrigin: string(value.BodyOrigin), Completeness: string(value.Completeness),
		WordCount: value.WordCount, Language: value.Language, Truncated: value.Truncated,
		QualityScore: copyDocumentFloat64(value.QualityScore), QualityWarnings: append([]string(nil), value.QualityWarnings...),
		ContentSHA256: value.ContentSHA256, ExtractorVersion: value.ExtractorVersion,
		ExtractorProfileVersion: value.ExtractorProfileVersion, ExtractorProfileSHA256: value.ExtractorProfileSHA256,
		DisplayPrivateRightsDecisionID: copyDocumentInt64(value.DisplayPrivateRightsDecisionID),
		LifecycleState:                 string(value.LifecycleState), CapturedAt: value.CapturedAt.UTC(),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func documentVersionDomainFromDTO(value DocumentVersionDTO) (ingestiondomain.DocumentVersion, error) {
	version := ingestiondomain.DocumentVersion{
		ID: value.ID, Version: value.Version, DocumentID: value.DocumentID,
		SourceObservationID: value.SourceObservationID, RevisionNo: value.RevisionNo,
		VersionKey: value.VersionKey, BodyOrigin: ingestiondomain.BodyOrigin(value.BodyOrigin),
		Completeness: ingestiondomain.BodyCompleteness(value.Completeness), WordCount: value.WordCount,
		Language: value.Language, Truncated: value.Truncated,
		QualityScore: copyDocumentFloat64(value.QualityScore), QualityWarnings: append([]string(nil), value.QualityWarnings...),
		ContentSHA256: value.ContentSHA256, ExtractorVersion: value.ExtractorVersion,
		ExtractorProfileVersion: value.ExtractorProfileVersion, ExtractorProfileSHA256: value.ExtractorProfileSHA256,
		DisplayPrivateRightsDecisionID: copyDocumentInt64(value.DisplayPrivateRightsDecisionID),
		LifecycleState:                 ingestiondomain.DocumentLifecycleState(value.LifecycleState),
		CapturedAt:                     value.CapturedAt.UTC(), CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
	if version.ID <= 0 || version.Version <= 0 || version.DocumentID <= 0 || version.SourceObservationID <= 0 ||
		version.RevisionNo <= 0 || !validLowerHexSHA256(version.VersionKey) || !version.BodyOrigin.Valid() ||
		!version.Completeness.Valid() || version.WordCount < 0 || version.Language == "" ||
		!validLowerHexSHA256(version.ContentSHA256) || version.ExtractorVersion == "" ||
		version.ExtractorProfileVersion == "" || !validLowerHexSHA256(version.ExtractorProfileSHA256) ||
		!ingestiondomain.DocumentVersionLifecycleStateValid(version.LifecycleState) || version.CapturedAt.IsZero() ||
		version.CreatedAt.IsZero() || version.UpdatedAt.IsZero() || version.UpdatedAt.Before(version.CreatedAt) {
		return ingestiondomain.DocumentVersion{}, fmt.Errorf("document version DTO is invalid")
	}
	return version, nil
}

func derivedArtifactDTOFromDomain(value ingestiondomain.DerivedArtifact) DerivedArtifactDTO {
	result := DerivedArtifactDTO{
		ID: value.ID, SourceConnectionID: value.SourceConnectionID, DocumentVersionID: value.DocumentVersionID,
		StoreDerivedRightsDecisionID: value.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       value.RetainRightsDecisionID,
		ArtifactType:                 string(value.ArtifactType), TransformerProfileSHA256: value.TransformerProfileSHA256,
		MIMEType: value.MIMEType, SHA256: value.SHA256, SizeBytes: value.SizeBytes,
		LifecycleState: string(value.LifecycleState), Active: value.Active, FailureCode: value.FailureCode,
		AvailableAt: copyDocumentTime(value.AvailableAt), RetentionUntil: value.RetentionUntil.UTC(),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
	if value.AnchorMap != nil {
		result.AnchorMap = &DerivedArtifactAnchorMapDTO{
			NormalizationVersion: value.AnchorMap.NormalizationVersion, AnchorMapProfileVersion: value.AnchorMap.AnchorMapProfileVersion,
			PlaintextSHA256: value.AnchorMap.PlaintextSHA256, MarkdownSHA256: value.AnchorMap.MarkdownSHA256, AnchorMapSHA256: value.AnchorMap.AnchorMapSHA256,
		}
	}
	return result
}

func derivedArtifactDomainFromDTO(value DerivedArtifactDTO) (ingestiondomain.DerivedArtifact, error) {
	artifact := ingestiondomain.DerivedArtifact{
		ID: value.ID, SourceConnectionID: value.SourceConnectionID, DocumentVersionID: value.DocumentVersionID,
		StoreDerivedRightsDecisionID: value.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       value.RetainRightsDecisionID,
		ArtifactType:                 ingestiondomain.DerivedArtifactType(value.ArtifactType),
		TransformerProfileSHA256:     value.TransformerProfileSHA256, MIMEType: value.MIMEType,
		SHA256: value.SHA256, SizeBytes: value.SizeBytes,
		LifecycleState: ingestiondomain.DerivedArtifactLifecycleState(value.LifecycleState),
		Active:         value.Active, FailureCode: value.FailureCode, AvailableAt: copyDocumentTime(value.AvailableAt),
		RetentionUntil: value.RetentionUntil.UTC(), CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
	if value.AnchorMap != nil {
		artifact.AnchorMap = &ingestiondomain.DocumentAnchorMapIdentity{
			NormalizationVersion: value.AnchorMap.NormalizationVersion, AnchorMapProfileVersion: value.AnchorMap.AnchorMapProfileVersion,
			PlaintextSHA256: value.AnchorMap.PlaintextSHA256, MarkdownSHA256: value.AnchorMap.MarkdownSHA256, AnchorMapSHA256: value.AnchorMap.AnchorMapSHA256,
		}
	}
	if err := artifact.Validate(); err != nil {
		return ingestiondomain.DerivedArtifact{}, err
	}
	return artifact, nil
}

func validApplicationBodyOrigin(value string) bool {
	return ingestiondomain.BodyOrigin(value).Valid()
}

func validApplicationBodyCompleteness(value string) bool {
	return ingestiondomain.BodyCompleteness(value).Valid()
}

func validApplicationDocumentLifecycle(value string) bool {
	return ingestiondomain.DocumentVersionLifecycleStateValid(ingestiondomain.DocumentLifecycleState(value))
}

func validApplicationDerivedArtifactType(value string) bool {
	return ingestiondomain.DerivedArtifactType(value).Valid()
}

func applicationDerivedArtifactMIMEType(value string) string {
	return ingestiondomain.DerivedArtifactType(value).MIMEType()
}

func copyDocumentString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyDocumentInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyDocumentFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyDocumentTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
