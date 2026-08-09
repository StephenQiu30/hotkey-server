package application

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const MaximumRawEvidenceReadBytes int64 = 4 << 20

type EvidenceSelectionQuery struct {
	EvidenceReferenceID int64
}

// EvidenceSelectionManifestDTO is the Source Application read model used to
// verify one immutable observation-to-snapshot locator. Object identity stays
// inside Source and is never copied into the public result.
type EvidenceSelectionManifestDTO struct {
	EvidenceReferenceID int64
	SourceObservationID int64
	EvidenceSnapshotID  int64
	SourceConnectionID  int64

	ExternalID       string
	UpstreamIdentity string
	SourceCode       string
	ContentType      string
	Title            string
	Language         string
	Author           string
	SourceRecordURL  string
	CanonicalURL     string
	DiscussionURL    string
	BodyOrigin       string
	Completeness     string
	PublishedAt      *time.Time
	DiscoveredAt     time.Time
	ObservationState string

	LifecycleState          string
	EvidenceKey             string
	ObjectKey               string
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
	SizeBytes               int64
	ResponseStatus          int
	RequestedURL            string
	FinalURL                string
	RedirectChain           []string
	ResponseHeaders         RawResponseHeadersDTO
	CapturedAt              time.Time
	RetentionUntil          time.Time
	StoreRawAllowed         bool
	RetainAllowed           bool
	CurrentRetentionDays    *int
	RightsEvaluatedAt       time.Time
	EvidenceReference       RawEvidenceReferenceDTO
}

type ReadRawEvidenceObjectQuery struct {
	SourceConnectionID      int64
	EvidenceKey             string
	ObjectKey               string
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
	SizeBytes               int64
	MaximumBytes            int64
}

func (query ReadRawEvidenceObjectQuery) Validate() error {
	if query.SourceConnectionID <= 0 || !validSHA256Hex(query.EvidenceKey) || !validSHA256Hex(query.PayloadSHA256) ||
		query.ObjectKey != RawEvidenceObjectKey(query.SourceConnectionID, query.EvidenceKey) || query.MIMEType == "" ||
		query.SizeBytes <= 0 || query.MaximumBytes <= 0 || query.SizeBytes > query.MaximumBytes || query.MaximumBytes > MaximumRawEvidenceReadBytes {
		return errors.New("raw evidence object read query is invalid")
	}
	if query.MIMEType != strings.TrimSpace(query.MIMEType) || len(query.MIMEType) > 255 || strings.ContainsAny(query.MIMEType, "\x00\r\n") {
		return errors.New("raw evidence object MIME type is invalid")
	}
	profile, err := domain.NewCollectorProfileVersion(query.CollectorProfileVersion)
	if err != nil {
		return errors.New("raw evidence object collector profile is invalid")
	}
	identity, err := domain.EvidenceSnapshotIdentity(query.PayloadSHA256, profile)
	if err != nil || identity != query.EvidenceKey {
		return errors.New("raw evidence object identity is invalid")
	}
	return nil
}

type ReadRawEvidenceObjectResult struct {
	Payload []byte
}

type EvidenceSelectionManifestReader interface {
	ReadEvidenceSelectionManifest(context.Context, int64) (EvidenceSelectionManifestDTO, error)
}

type RawEvidenceObjectReader interface {
	Read(context.Context, ReadRawEvidenceObjectQuery) (ReadRawEvidenceObjectResult, error)
}

type EvidenceByteSelector interface {
	Select(EvidenceSelectorInputDTO) ([]byte, error)
}

// SelectedEvidenceDTO is the only raw-derived payload allowed to cross the
// Source Application boundary. It has exact immutable IDs and provenance but
// no MinIO object key, bucket, response headers, or rights-decision IDs.
type SelectedEvidenceDTO struct {
	EvidenceReferenceID int64
	SourceObservationID int64
	EvidenceSnapshotID  int64
	SourceConnectionID  int64

	ExternalID       string
	UpstreamIdentity string
	SourceCode       string
	ContentType      string
	Title            string
	Language         string
	Author           string
	SourceRecordURL  string
	CanonicalURL     string
	DiscussionURL    string
	BodyOrigin       string
	Completeness     string
	PublishedAt      *time.Time
	DiscoveredAt     time.Time
	CapturedAt       time.Time

	SelectedPayload       []byte
	SelectedPayloadSHA256 string
	PayloadMIMEType       string
	SelectorVersion       string
}

type EvidenceSelectionResult struct {
	Evidence SelectedEvidenceDTO
}

type EvidenceSelectionDependencies struct {
	Manifests EvidenceSelectionManifestReader
	Objects   RawEvidenceObjectReader
	Selector  EvidenceByteSelector
}

type EvidenceSelectionService struct {
	manifests EvidenceSelectionManifestReader
	objects   RawEvidenceObjectReader
	selector  EvidenceByteSelector
}

func NewEvidenceSelectionService(dependencies EvidenceSelectionDependencies) (*EvidenceSelectionService, error) {
	if dependencies.Manifests == nil || dependencies.Objects == nil || dependencies.Selector == nil {
		return nil, errors.New("evidence selection manifest, object reader, and selector are required")
	}
	return &EvidenceSelectionService{manifests: dependencies.Manifests, objects: dependencies.Objects, selector: dependencies.Selector}, nil
}

func (service *EvidenceSelectionService) Read(ctx context.Context, query EvidenceSelectionQuery) (EvidenceSelectionResult, error) {
	if service == nil || service.manifests == nil || service.objects == nil || service.selector == nil || query.EvidenceReferenceID <= 0 {
		return EvidenceSelectionResult{}, fmt.Errorf("%w: invalid evidence selection query", sharedrepository.ErrInvalidInput)
	}
	manifest, err := service.manifests.ReadEvidenceSelectionManifest(ctx, query.EvidenceReferenceID)
	if err != nil {
		return EvidenceSelectionResult{}, fmt.Errorf("read evidence selection manifest: %w", err)
	}
	if err := validateEvidenceSelectionManifest(manifest, query); err != nil {
		return EvidenceSelectionResult{}, err
	}
	object, err := service.objects.Read(ctx, ReadRawEvidenceObjectQuery{
		SourceConnectionID: manifest.SourceConnectionID, EvidenceKey: manifest.EvidenceKey,
		ObjectKey: manifest.ObjectKey, PayloadSHA256: manifest.PayloadSHA256,
		CollectorProfileVersion: manifest.CollectorProfileVersion, MIMEType: manifest.MIMEType,
		SizeBytes: manifest.SizeBytes, MaximumBytes: MaximumRawEvidenceReadBytes,
	})
	if err != nil {
		return EvidenceSelectionResult{}, fmt.Errorf("read immutable raw evidence object: %w", err)
	}
	if int64(len(object.Payload)) != manifest.SizeBytes {
		return EvidenceSelectionResult{}, domain.ErrRawEvidenceConflict
	}
	snapshot, err := rawEvidenceSnapshotEntityFromDTO(RawEvidenceSnapshotDTO{
		EvidenceKey: manifest.EvidenceKey, Payload: object.Payload, PayloadSHA256: manifest.PayloadSHA256,
		CollectorProfileVersion: manifest.CollectorProfileVersion, MIMEType: manifest.MIMEType, ResponseStatus: manifest.ResponseStatus,
		RequestedURL: manifest.RequestedURL, FinalURL: manifest.FinalURL,
		RedirectChain: manifest.RedirectChain, ResponseHeaders: manifest.ResponseHeaders, CapturedAt: manifest.CapturedAt,
	})
	if err != nil || snapshot.Key != manifest.EvidenceKey {
		return EvidenceSelectionResult{}, domain.ErrRawEvidenceConflict
	}
	reference, err := rawEvidenceReferenceEntityFromDTO(manifest.EvidenceReference)
	if err != nil || reference.SnapshotKey != snapshot.Key {
		return EvidenceSelectionResult{}, domain.ErrRawEvidenceConflict
	}
	selected, err := service.selector.Select(evidenceSelectorInputDTOFromEntities(snapshot, reference))
	if err != nil {
		return EvidenceSelectionResult{}, fmt.Errorf("select immutable evidence bytes: %w", err)
	}
	digest := sha256.Sum256(selected)
	declaredDigest, decodeErr := hex.DecodeString(manifest.EvidenceReference.SelectedPayloadSHA256)
	if decodeErr != nil || len(declaredDigest) != sha256.Size || subtle.ConstantTimeCompare(digest[:], declaredDigest) != 1 {
		return EvidenceSelectionResult{}, domain.ErrEvidenceSelection
	}
	// Re-read the manifest after object access and selection so a concurrent
	// rights revocation or retention transition cannot race a successful read.
	currentManifest, err := service.manifests.ReadEvidenceSelectionManifest(ctx, query.EvidenceReferenceID)
	if err != nil {
		return EvidenceSelectionResult{}, fmt.Errorf("revalidate evidence selection manifest: %w", err)
	}
	if err := validateEvidenceSelectionManifest(currentManifest, query); err != nil {
		return EvidenceSelectionResult{}, err
	}
	if !sameEvidenceSelectionFacts(manifest, currentManifest) {
		return EvidenceSelectionResult{}, domain.ErrRawEvidenceConflict
	}
	return EvidenceSelectionResult{Evidence: selectedEvidenceDTO(manifest, selected)}, nil
}

func sameEvidenceSelectionFacts(left, right EvidenceSelectionManifestDTO) bool {
	return left.EvidenceReferenceID == right.EvidenceReferenceID &&
		left.SourceObservationID == right.SourceObservationID && left.EvidenceSnapshotID == right.EvidenceSnapshotID &&
		left.SourceConnectionID == right.SourceConnectionID && left.ExternalID == right.ExternalID &&
		left.UpstreamIdentity == right.UpstreamIdentity && left.SourceCode == right.SourceCode &&
		left.ContentType == right.ContentType && left.Title == right.Title && left.Language == right.Language &&
		left.Author == right.Author && left.SourceRecordURL == right.SourceRecordURL &&
		left.CanonicalURL == right.CanonicalURL && left.DiscussionURL == right.DiscussionURL &&
		left.BodyOrigin == right.BodyOrigin && left.Completeness == right.Completeness &&
		equalOptionalTime(left.PublishedAt, right.PublishedAt) && left.DiscoveredAt.Equal(right.DiscoveredAt) &&
		left.ObservationState == right.ObservationState && left.LifecycleState == right.LifecycleState &&
		left.EvidenceKey == right.EvidenceKey &&
		left.ObjectKey == right.ObjectKey && left.PayloadSHA256 == right.PayloadSHA256 &&
		left.CollectorProfileVersion == right.CollectorProfileVersion && left.MIMEType == right.MIMEType &&
		left.SizeBytes == right.SizeBytes && left.ResponseStatus == right.ResponseStatus &&
		left.RequestedURL == right.RequestedURL && left.FinalURL == right.FinalURL &&
		strings.Join(left.RedirectChain, "\x00") == strings.Join(right.RedirectChain, "\x00") &&
		left.ResponseHeaders.Equal(right.ResponseHeaders) && left.CapturedAt.Equal(right.CapturedAt) &&
		left.RetentionUntil.Equal(right.RetentionUntil) &&
		sameEvidenceReference(left.EvidenceReference, right.EvidenceReference)
}

func sameEvidenceReference(left, right RawEvidenceReferenceDTO) bool {
	return left.EvidenceKey == right.EvidenceKey && left.LocatorType == right.LocatorType &&
		left.LocatorValue == right.LocatorValue && equalOptionalInt64(left.ByteStart, right.ByteStart) &&
		equalOptionalInt64(left.ByteEnd, right.ByteEnd) &&
		left.SelectedPayloadSHA256 == right.SelectedPayloadSHA256 && left.SelectorVersion == right.SelectorVersion
}

func equalOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func validateEvidenceSelectionManifest(manifest EvidenceSelectionManifestDTO, query EvidenceSelectionQuery) error {
	lifecycleState, lifecycleErr := evidenceLifecycleEntityFromString(manifest.LifecycleState)
	if manifest.EvidenceReferenceID != query.EvidenceReferenceID || manifest.SourceObservationID <= 0 ||
		manifest.EvidenceSnapshotID <= 0 || manifest.SourceConnectionID <= 0 || !validEvidenceSelectionText(manifest.ExternalID, 512) ||
		!validSHA256Hex(manifest.UpstreamIdentity) || !validEvidenceSelectionText(manifest.SourceCode, 64) ||
		!validEvidenceSelectionText(manifest.ContentType, 32) || !validEvidenceSelectionText(manifest.Language, 32) ||
		len(manifest.Title) > 1<<20 || len(manifest.Author) > 512 ||
		!validEvidenceBodyOrigin(manifest.BodyOrigin) || !validEvidenceCompleteness(manifest.Completeness) ||
		!validEvidenceSelectionURLs(manifest) || manifest.ResponseStatus < 100 || manifest.ResponseStatus > 599 ||
		manifest.CapturedAt.IsZero() ||
		manifest.DiscoveredAt.IsZero() || manifest.CapturedAt.Before(manifest.DiscoveredAt) || manifest.RightsEvaluatedAt.IsZero() ||
		lifecycleErr != nil || lifecycleState != domain.EvidenceLifecycleAvailable ||
		(manifest.ObservationState != "active" && manifest.ObservationState != "corrected") ||
		!manifest.StoreRawAllowed || !manifest.RetainAllowed || manifest.CurrentRetentionDays == nil ||
		*manifest.CurrentRetentionDays <= 0 || *manifest.CurrentRetentionDays > 3650 ||
		!manifest.RetentionUntil.After(manifest.RightsEvaluatedAt) ||
		manifest.RetentionUntil.After(manifest.CapturedAt.Add(time.Duration(*manifest.CurrentRetentionDays)*24*time.Hour)) {
		return fmt.Errorf("%w: evidence selection is not currently readable", sharedrepository.ErrConstraint)
	}
	objectQuery := ReadRawEvidenceObjectQuery{
		SourceConnectionID: manifest.SourceConnectionID, EvidenceKey: manifest.EvidenceKey,
		ObjectKey: manifest.ObjectKey, PayloadSHA256: manifest.PayloadSHA256,
		CollectorProfileVersion: manifest.CollectorProfileVersion, MIMEType: manifest.MIMEType,
		SizeBytes: manifest.SizeBytes, MaximumBytes: MaximumRawEvidenceReadBytes,
	}
	if err := objectQuery.Validate(); err != nil {
		return fmt.Errorf("%w: evidence selection object manifest is invalid", sharedrepository.ErrConstraint)
	}
	reference, err := rawEvidenceReferenceEntityFromDTO(manifest.EvidenceReference)
	if err != nil || reference.SnapshotKey != manifest.EvidenceKey {
		return fmt.Errorf("%w: evidence selection locator is invalid", sharedrepository.ErrConstraint)
	}
	return nil
}

func validEvidenceSelectionText(value string, maximumBytes int) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maximumBytes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validEvidenceSelectionURLs(manifest EvidenceSelectionManifestDTO) bool {
	values := []string{manifest.SourceRecordURL, manifest.CanonicalURL, manifest.DiscussionURL}
	found := false
	for _, value := range values {
		if value == "" {
			continue
		}
		found = true
		if !validEvidenceSelectionURL(value) {
			return false
		}
	}
	return found
}

func validEvidenceSelectionURL(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 2048 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && parsed.User == nil && parsed.Fragment == "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != ""
}

func validEvidenceBodyOrigin(value string) bool {
	switch value {
	case "api_content", "feed_content", "feed_summary", "structured_article_body", "authorized_payload_extraction", "platform_post", "search_snippet":
		return true
	default:
		return false
	}
}

func validEvidenceCompleteness(value string) bool {
	switch value {
	case "full", "partial", "summary", "snippet", "metadata_only", "unknown":
		return true
	default:
		return false
	}
}

func selectedEvidenceDTO(manifest EvidenceSelectionManifestDTO, selected []byte) SelectedEvidenceDTO {
	return SelectedEvidenceDTO{
		EvidenceReferenceID: manifest.EvidenceReferenceID, SourceObservationID: manifest.SourceObservationID,
		EvidenceSnapshotID: manifest.EvidenceSnapshotID, SourceConnectionID: manifest.SourceConnectionID,
		ExternalID: manifest.ExternalID, UpstreamIdentity: manifest.UpstreamIdentity,
		SourceCode: manifest.SourceCode, ContentType: manifest.ContentType, Title: manifest.Title,
		Language: manifest.Language, Author: manifest.Author, SourceRecordURL: manifest.SourceRecordURL,
		CanonicalURL: manifest.CanonicalURL, DiscussionURL: manifest.DiscussionURL,
		BodyOrigin: manifest.BodyOrigin, Completeness: manifest.Completeness,
		PublishedAt: copyTime(manifest.PublishedAt), DiscoveredAt: manifest.DiscoveredAt.UTC(), CapturedAt: manifest.CapturedAt.UTC(),
		SelectedPayload: append([]byte(nil), selected...), SelectedPayloadSHA256: manifest.EvidenceReference.SelectedPayloadSHA256,
		PayloadMIMEType: manifest.MIMEType, SelectorVersion: manifest.EvidenceReference.SelectorVersion,
	}
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
