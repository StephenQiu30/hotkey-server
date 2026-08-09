package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// DocumentObservationReader is the narrow Source Application read port used
// by document persistence. Its implementation must return only authorized
// captured text; Ingestion does not read Source tables or fetch canonical URLs.
type DocumentObservationReader interface {
	ReadDocumentObservation(context.Context, int64) (DocumentObservationDTO, error)
}

type DocumentObservationDTO struct {
	ID                 int64
	SourceConnectionID int64
	ExternalWorkID     string
	BodyOrigin         string
	Completeness       string
	Body               string
	Language           string
	CapturedAt         time.Time
}

type PersistDocumentVersionCommand struct {
	SourceObservationID     int64
	ExtractorVersion        string
	ExtractorProfileVersion string
	ExtractorProfileSHA256  string
	Truncated               bool
	QualityScore            *float64
	QualityWarnings         []string
}

// PersistDocumentObservationCommand carries one Source observation that has
// already been reverified by an upstream Application reader. The body exists
// only at this in-memory Application boundary and is never copied into the
// persistence DTO.
type PersistDocumentObservationCommand struct {
	Observation             DocumentObservationDTO
	ExtractorVersion        string
	ExtractorProfileVersion string
	ExtractorProfileSHA256  string
	Truncated               bool
	QualityScore            *float64
	QualityWarnings         []string
}

type PersistDocumentVersionResult struct {
	Document               DocumentDTO
	DocumentCreated        bool
	DocumentVersion        DocumentVersionDTO
	DocumentVersionCreated bool
}

// DocumentVersionDraftDTO carries immutable normalized metadata to the
// persistence port. It intentionally has no document body field.
type DocumentVersionDraftDTO struct {
	SourceConnectionID      int64
	DocumentID              int64
	SourceObservationID     int64
	VersionKey              string
	BodyOrigin              string
	Completeness            string
	WordCount               int
	Language                string
	Truncated               bool
	QualityScore            *float64
	QualityWarnings         []string
	ContentSHA256           string
	ExtractorVersion        string
	ExtractorProfileVersion string
	ExtractorProfileSHA256  string
	LifecycleState          string
	CapturedAt              time.Time
}

type DocumentVersionRepository interface {
	ResolveDocument(context.Context, DocumentIdentityDTO) (DocumentDTO, bool, error)
	AppendDocumentVersion(context.Context, DocumentVersionDraftDTO) (DocumentVersionDTO, bool, error)
	GetDocumentVersion(context.Context, int64) (DocumentVersionDTO, error)
	CompareAndSwapDocumentVersionLifecycle(context.Context, TransitionDocumentVersionCommand) (DocumentVersionDTO, error)
}

type TransitionDocumentVersionCommand struct {
	DocumentVersionID              int64
	ExpectedVersion                int64
	To                             string
	DisplayPrivateRightsDecisionID *int64
}

type TransitionDocumentVersionResult struct {
	DocumentVersion DocumentVersionDTO
}

func ValidateTransitionDocumentVersionCommand(command TransitionDocumentVersionCommand) error {
	transition := ingestiondomain.DocumentVersionTransition{
		DocumentVersionID: command.DocumentVersionID, ExpectedVersion: command.ExpectedVersion,
		To:                             ingestiondomain.DocumentLifecycleState(command.To),
		DisplayPrivateRightsDecisionID: command.DisplayPrivateRightsDecisionID,
	}
	return transition.Validate()
}

// DocumentVersionUseCases is the application boundary consumed by workers.
// It is transport-neutral and never returns document body bytes.
type DocumentVersionUseCases interface {
	PersistSourceObservation(context.Context, PersistDocumentVersionCommand) (PersistDocumentVersionResult, error)
	PersistDocumentObservation(context.Context, PersistDocumentObservationCommand) (PersistDocumentVersionResult, error)
	TransitionDocumentVersion(context.Context, TransitionDocumentVersionCommand) (TransitionDocumentVersionResult, error)
}

type DocumentVersionDependencies struct {
	Observations DocumentObservationReader
	Versions     DocumentVersionRepository
}

type DocumentVersionService struct {
	observations DocumentObservationReader
	versions     DocumentVersionRepository
}

var _ DocumentVersionUseCases = (*DocumentVersionService)(nil)

func NewDocumentVersionService(dependencies DocumentVersionDependencies) (*DocumentVersionService, error) {
	if dependencies.Observations == nil || dependencies.Versions == nil {
		return nil, errors.New("document version application dependencies are required")
	}
	return &DocumentVersionService{observations: dependencies.Observations, versions: dependencies.Versions}, nil
}

// NewDocumentObservationPersistenceService constructs the write side used by
// workers that already hold a reverified DocumentObservationDTO. It does not
// install a placeholder Source reader; the legacy read-and-persist method
// therefore remains unavailable on this instance.
func NewDocumentObservationPersistenceService(versions DocumentVersionRepository) (*DocumentVersionService, error) {
	if versions == nil {
		return nil, errors.New("document version repository is required")
	}
	return &DocumentVersionService{versions: versions}, nil
}

func (service *DocumentVersionService) PersistSourceObservation(ctx context.Context, command PersistDocumentVersionCommand) (PersistDocumentVersionResult, error) {
	if service == nil || service.observations == nil || service.versions == nil || command.SourceObservationID <= 0 {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: invalid document version input", sharedrepository.ErrInvalidInput)
	}
	observation, err := service.observations.ReadDocumentObservation(ctx, command.SourceObservationID)
	if err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("read authorized source observation: %w", err)
	}
	if observation.ID != command.SourceObservationID {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: source observation projection does not match request", sharedrepository.ErrInvalidInput)
	}
	return service.PersistDocumentObservation(ctx, PersistDocumentObservationCommand{
		Observation: observation, ExtractorVersion: command.ExtractorVersion,
		ExtractorProfileVersion: command.ExtractorProfileVersion, ExtractorProfileSHA256: command.ExtractorProfileSHA256,
		Truncated: command.Truncated, QualityScore: cloneDocumentQualityScore(command.QualityScore),
		QualityWarnings: append([]string(nil), command.QualityWarnings...),
	})
}

// PersistDocumentObservation appends one immutable version without rereading
// Source. Callers are responsible for supplying a currently authorized,
// already-verified observation projection.
func (service *DocumentVersionService) PersistDocumentObservation(ctx context.Context, command PersistDocumentObservationCommand) (PersistDocumentVersionResult, error) {
	if service == nil || service.versions == nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: invalid document observation input", sharedrepository.ErrInvalidInput)
	}
	observation := command.Observation
	if observation.ID <= 0 || observation.SourceConnectionID <= 0 || observation.CapturedAt.IsZero() {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: source observation projection is invalid", sharedrepository.ErrInvalidInput)
	}
	persistCommand := PersistDocumentVersionCommand{
		SourceObservationID: observation.ID, ExtractorVersion: command.ExtractorVersion,
		ExtractorProfileVersion: command.ExtractorProfileVersion, ExtractorProfileSHA256: command.ExtractorProfileSHA256,
		Truncated: command.Truncated, QualityScore: cloneDocumentQualityScore(command.QualityScore),
		QualityWarnings: append([]string(nil), command.QualityWarnings...),
	}

	// Validate body and extractor semantics before creating the stable
	// Document container. Normalize is repeated after resolving its real ID
	// because DocumentID participates in the immutable VersionKey.
	validationCandidate := documentVersionCandidate(1, observation, persistCommand)
	if _, err := validationCandidate.Normalize(); err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	documentIdentity, err := documentIdentityFromObservationDTO(observation)
	if err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	documentDTO, documentCreated, err := service.versions.ResolveDocument(ctx, documentIdentityDTOFromDomain(documentIdentity))
	if err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("resolve document identity: %w", err)
	}
	document, err := documentDomainFromDTO(documentDTO)
	if err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: resolved document is invalid", sharedrepository.ErrConflict)
	}
	if !resolvedDocumentMatches(document, documentIdentity) {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: resolved document identity changed", sharedrepository.ErrConflict)
	}

	normalized, err := documentVersionCandidate(document.ID, observation, persistCommand).Normalize()
	if err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: normalize document version: %v", sharedrepository.ErrInvalidInput, err)
	}
	storedDTO, versionCreated, err := service.versions.AppendDocumentVersion(ctx, newDocumentVersionDraftDTO(observation.SourceConnectionID, normalized))
	if err != nil {
		return PersistDocumentVersionResult{}, fmt.Errorf("append immutable document version: %w", err)
	}
	stored, err := documentVersionDomainFromDTO(storedDTO)
	if err != nil || stored.DocumentID != document.ID || stored.SourceObservationID != observation.ID {
		return PersistDocumentVersionResult{}, fmt.Errorf("%w: appended document version is invalid", sharedrepository.ErrConflict)
	}
	return PersistDocumentVersionResult{
		Document: documentDTOFromDomain(document), DocumentCreated: documentCreated,
		DocumentVersion: documentVersionDTOFromDomain(stored), DocumentVersionCreated: versionCreated,
	}, nil
}

func newDocumentVersionDraftDTO(sourceConnectionID int64, normalized ingestiondomain.NormalizedDocumentVersion) DocumentVersionDraftDTO {
	return DocumentVersionDraftDTO{
		SourceConnectionID: sourceConnectionID,
		DocumentID:         normalized.DocumentID, SourceObservationID: normalized.SourceObservationID,
		VersionKey: normalized.VersionKey, BodyOrigin: string(normalized.BodyOrigin), Completeness: string(normalized.Completeness),
		WordCount: normalized.WordCount, Language: normalized.Language, Truncated: normalized.Truncated,
		QualityScore:    cloneDocumentQualityScore(normalized.QualityScore),
		QualityWarnings: append([]string{}, normalized.QualityWarnings...),
		ContentSHA256:   normalized.ContentSHA256, ExtractorVersion: normalized.ExtractorVersion,
		ExtractorProfileVersion: normalized.ExtractorProfileVersion,
		ExtractorProfileSHA256:  normalized.ExtractorProfileSHA256,
		LifecycleState:          string(normalized.LifecycleState), CapturedAt: normalized.CapturedAt,
	}
}

func ValidateDocumentVersionDraftDTO(draft DocumentVersionDraftDTO) error {
	if draft.SourceConnectionID <= 0 || draft.DocumentID <= 0 || draft.SourceObservationID <= 0 ||
		!validLowerHexSHA256(draft.VersionKey) || !validLowerHexSHA256(draft.ContentSHA256) ||
		!validApplicationBodyOrigin(draft.BodyOrigin) || !validApplicationBodyCompleteness(draft.Completeness) || draft.WordCount < 0 ||
		strings.TrimSpace(draft.Language) == "" || utf8.RuneCountInString(draft.Language) > 32 || draft.CapturedAt.IsZero() ||
		draft.LifecycleState != DocumentPolicyPending {
		return fmt.Errorf("document version draft is invalid")
	}
	if strings.TrimSpace(draft.ExtractorVersion) != draft.ExtractorVersion || draft.ExtractorVersion == "" || utf8.RuneCountInString(draft.ExtractorVersion) > 64 ||
		strings.TrimSpace(draft.ExtractorProfileVersion) != draft.ExtractorProfileVersion || draft.ExtractorProfileVersion == "" || utf8.RuneCountInString(draft.ExtractorProfileVersion) > 64 ||
		!validLowerHexSHA256(draft.ExtractorProfileSHA256) {
		return fmt.Errorf("document version extractor metadata is invalid")
	}
	if draft.QualityScore != nil && (math.IsNaN(*draft.QualityScore) || math.IsInf(*draft.QualityScore, 0) || *draft.QualityScore < 0 || *draft.QualityScore > 100) {
		return fmt.Errorf("document version quality score is invalid")
	}
	if len(draft.QualityWarnings) > 32 || !slices.IsSorted(draft.QualityWarnings) {
		return fmt.Errorf("document version quality warnings are invalid")
	}
	for index, warning := range draft.QualityWarnings {
		if warning == "" || len(warning) > 64 || strings.TrimSpace(warning) != warning || strings.ToLower(warning) != warning ||
			(index > 0 && warning == draft.QualityWarnings[index-1]) {
			return fmt.Errorf("document version quality warnings are invalid")
		}
	}
	return nil
}

func validLowerHexSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}

func cloneDocumentQualityScore(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (service *DocumentVersionService) TransitionDocumentVersion(ctx context.Context, command TransitionDocumentVersionCommand) (TransitionDocumentVersionResult, error) {
	transition := ingestiondomain.DocumentVersionTransition{
		DocumentVersionID: command.DocumentVersionID, ExpectedVersion: command.ExpectedVersion,
		To:                             ingestiondomain.DocumentLifecycleState(command.To),
		DisplayPrivateRightsDecisionID: command.DisplayPrivateRightsDecisionID,
	}
	if service == nil || service.versions == nil {
		return TransitionDocumentVersionResult{}, fmt.Errorf("%w: invalid document version lifecycle input", sharedrepository.ErrInvalidInput)
	}
	if err := transition.Validate(); err != nil {
		return TransitionDocumentVersionResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	currentDTO, err := service.versions.GetDocumentVersion(ctx, command.DocumentVersionID)
	if err != nil {
		return TransitionDocumentVersionResult{}, err
	}
	current, err := documentVersionDomainFromDTO(currentDTO)
	if err != nil {
		return TransitionDocumentVersionResult{}, fmt.Errorf("%w: current document version is invalid", sharedrepository.ErrConflict)
	}
	if current.Version != command.ExpectedVersion {
		return TransitionDocumentVersionResult{}, fmt.Errorf("%w: document version changed", sharedrepository.ErrConflict)
	}
	if current.LifecycleState == transition.To {
		return TransitionDocumentVersionResult{DocumentVersion: documentVersionDTOFromDomain(current)}, nil
	}
	if err := ingestiondomain.ValidateDocumentTransition(current.LifecycleState, transition.To); err != nil {
		return TransitionDocumentVersionResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConflict, err)
	}
	updatedDTO, err := service.versions.CompareAndSwapDocumentVersionLifecycle(ctx, command)
	if err != nil {
		return TransitionDocumentVersionResult{}, err
	}
	updated, err := documentVersionDomainFromDTO(updatedDTO)
	if err != nil || updated.ID != command.DocumentVersionID || updated.Version != command.ExpectedVersion+1 || updated.LifecycleState != transition.To {
		return TransitionDocumentVersionResult{}, fmt.Errorf("%w: document version CAS returned inconsistent state", sharedrepository.ErrConflict)
	}
	return TransitionDocumentVersionResult{DocumentVersion: documentVersionDTOFromDomain(updated)}, nil
}

func documentVersionCandidate(documentID int64, observation DocumentObservationDTO, command PersistDocumentVersionCommand) ingestiondomain.DocumentVersionCandidate {
	return ingestiondomain.DocumentVersionCandidate{
		DocumentID: documentID, SourceObservationID: observation.ID,
		BodyOrigin: ingestiondomain.BodyOrigin(observation.BodyOrigin), Completeness: ingestiondomain.BodyCompleteness(observation.Completeness),
		Body: observation.Body, Language: observation.Language, CapturedAt: observation.CapturedAt,
		ExtractorVersion: command.ExtractorVersion, ExtractorProfileVersion: command.ExtractorProfileVersion,
		ExtractorProfileSHA256: command.ExtractorProfileSHA256, Truncated: command.Truncated,
		QualityScore: command.QualityScore, QualityWarnings: command.QualityWarnings,
	}
}

func documentIdentityFromObservationDTO(observation DocumentObservationDTO) (ingestiondomain.DocumentIdentity, error) {
	externalWorkID := ingestiondomain.NormalizeExternalID(observation.ExternalWorkID)
	if utf8.RuneCountInString(externalWorkID) > 512 {
		return ingestiondomain.DocumentIdentity{}, fmt.Errorf("external work identity is too long")
	}
	identityKind := "source-observation-v1"
	identityValue := fmt.Sprintf("%d", observation.ID)
	var persistedExternalWorkID *string
	if externalWorkID != "" {
		identityKind = "source-external-work-v1"
		identityValue = externalWorkID
		persistedExternalWorkID = &externalWorkID
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		identityKind, fmt.Sprintf("%d", observation.SourceConnectionID), identityValue,
	}, "\n")))
	return ingestiondomain.DocumentIdentity{
		SourceConnectionID: observation.SourceConnectionID,
		DocumentKey:        fmt.Sprintf("%x", digest),
		ExternalWorkID:     persistedExternalWorkID,
	}, nil
}

func resolvedDocumentMatches(document ingestiondomain.Document, identity ingestiondomain.DocumentIdentity) bool {
	return document.ID > 0 && document.Version > 0 && document.State == ingestiondomain.DocumentStateActive &&
		document.SourceConnectionID == identity.SourceConnectionID && document.DocumentKey == identity.DocumentKey &&
		optionalStringsEqual(document.ExternalWorkID, identity.ExternalWorkID)
}

func optionalStringsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
