package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"golang.org/x/text/unicode/norm"
)

const (
	derivedArtifactVaultFailureCode  = "vault_publish_failed"
	derivedArtifactCommitFailureCode = "artifact_commit_failed"
)

var ErrDerivedArtifactContentConflict = errors.New("immutable derived artifact content conflict")

const (
	DocumentProjectionMarkdown  = "markdown"
	DocumentProjectionPlaintext = "plaintext"
)

func validDocumentProjectionFormat(format string) bool {
	return format == DocumentProjectionMarkdown || format == DocumentProjectionPlaintext
}

type ProjectDocumentCommand struct {
	DocumentVersionID              int64
	ExpectedDocumentVersion        int64
	ArtifactType                   string
	TransformerProfileSHA256       string
	StoreDerivedRightsDecisionID   int64
	RetainRightsDecisionID         int64
	DisplayPrivateRightsDecisionID *int64
	ProjectionBytes                []byte
	AnchorMap                      *ProjectDocumentAnchorMapCommand
}

// ProjectDocumentAnchorMapCommand is the only body-bearing anchor command.
// Reserve/Commit persistence DTOs below deliberately strip Plaintext.
type ProjectDocumentAnchorMapCommand struct {
	Plaintext               string
	NormalizationVersion    string
	AnchorMapProfileVersion string
	PlaintextSHA256         string
	MarkdownSHA256          string
	AnchorMapSHA256         string
	Blocks                  []DocumentAnchorBlockDTO
}

type ProjectDocumentResult struct {
	Artifact        DerivedArtifactDTO
	DocumentVersion DocumentVersionDTO
}

type ReserveDerivedArtifactCommand struct {
	DocumentVersionID            int64
	ExpectedDocumentVersion      int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	ArtifactType                 string
	TransformerProfileSHA256     string
	MIMEType                     string
	SHA256                       string
	SizeBytes                    int64
	AnchorMap                    *DerivedArtifactAnchorMapDTO
}

type ReserveDerivedArtifactResult struct {
	Artifact          DerivedArtifactDTO
	DocumentID        int64
	VaultRelativePath string
	DocumentVersion   DocumentVersionDTO
}

type ProjectionReceiptDTO struct {
	DocumentID               int64
	DocumentVersionID        int64
	ArtifactType             string
	TransformerProfileSHA256 string
	VaultRelativePath        string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
}

type CommitDerivedArtifactCommand struct {
	ArtifactID   int64
	Receipt      ProjectionReceiptDTO
	AnchorBlocks []DocumentAnchorBlockDTO
}

type CommitDerivedArtifactResult struct {
	Artifact        DerivedArtifactDTO
	DocumentID      int64
	DocumentVersion DocumentVersionDTO
}

type MarkDerivedArtifactFailedCommand struct {
	ArtifactID  int64
	FailureCode string
}

type QuarantineDerivedArtifactCommand struct {
	ArtifactID int64
}

type DerivedArtifactRepository interface {
	Reserve(context.Context, ReserveDerivedArtifactCommand) (ReserveDerivedArtifactResult, error)
	Commit(context.Context, CommitDerivedArtifactCommand) (CommitDerivedArtifactResult, error)
	MarkFailed(context.Context, MarkDerivedArtifactFailedCommand) (DerivedArtifactDTO, error)
	Quarantine(context.Context, QuarantineDerivedArtifactCommand) (DerivedArtifactDTO, error)
}

type DocumentVersionLifecycle interface {
	TransitionDocumentVersion(context.Context, TransitionDocumentVersionCommand) (TransitionDocumentVersionResult, error)
}

type DocumentProjectionDependencies struct {
	Publisher        knowledgeapplication.ProjectionPublisher
	Repository       DerivedArtifactRepository
	DocumentVersions DocumentVersionLifecycle
}

type DocumentProjectionService struct {
	publisher        knowledgeapplication.ProjectionPublisher
	repository       DerivedArtifactRepository
	documentVersions DocumentVersionLifecycle
}

func NewDocumentProjectionService(dependencies DocumentProjectionDependencies) (*DocumentProjectionService, error) {
	if dependencies.Publisher == nil || dependencies.Repository == nil || dependencies.DocumentVersions == nil {
		return nil, errors.New("document projection publisher, repository, and lifecycle are required")
	}
	return &DocumentProjectionService{
		publisher: dependencies.Publisher, repository: dependencies.Repository, documentVersions: dependencies.DocumentVersions,
	}, nil
}

func (service *DocumentProjectionService) Project(ctx context.Context, command ProjectDocumentCommand) (ProjectDocumentResult, error) {
	if service == nil || service.publisher == nil || service.repository == nil || service.documentVersions == nil {
		return ProjectDocumentResult{}, errors.New("document projection service is not initialized")
	}
	prepared, err := prepareDocumentProjection(command)
	if err != nil {
		return ProjectDocumentResult{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	reservation, err := service.repository.Reserve(ctx, prepared.reservation)
	if err != nil {
		if errors.Is(err, ErrDerivedArtifactContentConflict) && reservation.Artifact.ID > 0 {
			return ProjectDocumentResult{}, service.quarantineContentConflict(ctx, reservation)
		}
		return ProjectDocumentResult{}, fmt.Errorf("reserve derived artifact: %w", err)
	}
	if err := validateDerivedArtifactReservation(reservation, prepared.reservation); err != nil {
		if errors.Is(err, ErrDerivedArtifactContentConflict) && reservation.Artifact.ID > 0 && reservation.DocumentVersion.ID > 0 {
			return ProjectDocumentResult{}, service.quarantineContentConflict(ctx, reservation)
		}
		return ProjectDocumentResult{}, err
	}

	published, err := service.publisher.Publish(ctx, publishProjectionCommand(reservation, prepared, command.ProjectionBytes))
	if err != nil {
		if projectionContentConflict(err) {
			return ProjectDocumentResult{}, service.quarantineContentConflict(ctx, reservation)
		}
		return ProjectDocumentResult{}, service.failProjection(ctx, reservation, derivedArtifactVaultFailureCode, err)
	}
	receipt, err := projectionReceiptFromPublishResult(published, reservation, prepared.reservation)
	if err != nil {
		return ProjectDocumentResult{}, service.quarantineContentConflict(ctx, reservation)
	}
	committed, err := service.repository.Commit(ctx, CommitDerivedArtifactCommand{
		ArtifactID: reservation.Artifact.ID, Receipt: receipt, AnchorBlocks: cloneDocumentAnchorBlocks(prepared.anchorBlocks),
	})
	if err != nil {
		if errors.Is(err, ErrDerivedArtifactContentConflict) && committed.Artifact.ID > 0 {
			conflicting := reservation
			conflicting.Artifact = committed.Artifact
			conflicting.DocumentID = committed.DocumentID
			conflicting.DocumentVersion = committed.DocumentVersion
			return ProjectDocumentResult{}, service.quarantineContentConflict(ctx, conflicting)
		}
		return ProjectDocumentResult{}, service.failProjection(ctx, reservation, derivedArtifactCommitFailureCode, err)
	}
	if err := validateCommittedDerivedArtifact(committed, reservation, prepared.reservation); err != nil {
		if errors.Is(err, ErrDerivedArtifactContentConflict) {
			return ProjectDocumentResult{}, service.quarantineContentConflict(ctx, reservation)
		}
		return ProjectDocumentResult{}, err
	}

	committedArtifact, artifactErr := derivedArtifactDomainFromDTO(committed.Artifact)
	committedDocumentVersion, versionErr := documentVersionDomainFromDTO(committed.DocumentVersion)
	if artifactErr != nil || versionErr != nil {
		return ProjectDocumentResult{}, fmt.Errorf("%w: committed projection DTO is invalid", sharedrepository.ErrConflict)
	}
	documentVersion, err := service.advanceDocumentVersion(ctx, committedDocumentVersion, command.DisplayPrivateRightsDecisionID)
	if err != nil {
		return ProjectDocumentResult{}, fmt.Errorf("advance projected document lifecycle: %w", err)
	}
	return projectDocumentResult(committedArtifact, documentVersion), nil
}

func ValidateReserveDerivedArtifactCommand(command ReserveDerivedArtifactCommand) error {
	artifactType := ingestiondomain.DerivedArtifactType(command.ArtifactType)
	if command.DocumentVersionID <= 0 || command.ExpectedDocumentVersion <= 0 ||
		command.StoreDerivedRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		!artifactType.Valid() || !validLowerHexSHA256(command.TransformerProfileSHA256) ||
		command.MIMEType != artifactType.MIMEType() || !validLowerHexSHA256(command.SHA256) ||
		command.SizeBytes <= 0 {
		return errors.New("derived artifact reservation command is invalid")
	}
	if command.ArtifactType == DocumentProjectionMarkdown {
		if err := validateDerivedArtifactAnchorMapDTO(command.AnchorMap, command.SHA256); err != nil {
			return err
		}
	} else if command.AnchorMap != nil {
		return errors.New("plaintext artifact reservation cannot carry an anchor map")
	}
	return nil
}

func ValidateProjectionReceiptDTO(receipt ProjectionReceiptDTO) error {
	artifactType := ingestiondomain.DerivedArtifactType(receipt.ArtifactType)
	if receipt.DocumentID <= 0 || receipt.DocumentVersionID <= 0 || !artifactType.Valid() ||
		!validLowerHexSHA256(receipt.TransformerProfileSHA256) || receipt.MIMEType != artifactType.MIMEType() ||
		!validLowerHexSHA256(receipt.SHA256) || receipt.SizeBytes <= 0 ||
		receipt.VaultRelativePath != documentProjectionRelativePath(
			receipt.DocumentID, receipt.DocumentVersionID, receipt.ArtifactType, receipt.TransformerProfileSHA256,
		) {
		return errors.New("projection receipt is invalid")
	}
	return nil
}

type preparedDocumentProjection struct {
	reservation  ReserveDerivedArtifactCommand
	format       knowledgeapplication.ProjectionFormat
	anchorBlocks []DocumentAnchorBlockDTO
}

func prepareDocumentProjection(command ProjectDocumentCommand) (preparedDocumentProjection, error) {
	if command.DocumentVersionID <= 0 || command.ExpectedDocumentVersion <= 0 || !validDocumentProjectionFormat(command.ArtifactType) ||
		command.StoreDerivedRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		!validLowerHexSHA256(command.TransformerProfileSHA256) || len(command.ProjectionBytes) == 0 ||
		!utf8.Valid(command.ProjectionBytes) || strings.ContainsRune(string(command.ProjectionBytes), '\r') ||
		norm.NFC.String(string(command.ProjectionBytes)) != string(command.ProjectionBytes) {
		return preparedDocumentProjection{}, errors.New("document projection command is invalid")
	}
	if command.DisplayPrivateRightsDecisionID != nil && *command.DisplayPrivateRightsDecisionID <= 0 {
		return preparedDocumentProjection{}, errors.New("display-private rights decision is invalid")
	}
	if command.ArtifactType == DocumentProjectionMarkdown {
		if command.AnchorMap == nil {
			return preparedDocumentProjection{}, errors.New("markdown projection requires an immutable anchor map")
		}
		mapped := MapDocumentTextResult{
			Plaintext: command.AnchorMap.Plaintext, NormalizationVersion: command.AnchorMap.NormalizationVersion,
			AnchorMapProfileVersion: command.AnchorMap.AnchorMapProfileVersion,
			PlaintextSHA256:         command.AnchorMap.PlaintextSHA256, MarkdownSHA256: command.AnchorMap.MarkdownSHA256,
			AnchorMapSHA256: command.AnchorMap.AnchorMapSHA256, Blocks: cloneDocumentAnchorBlocks(command.AnchorMap.Blocks),
		}
		if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: string(command.ProjectionBytes)}, mapped); err != nil {
			return preparedDocumentProjection{}, fmt.Errorf("markdown anchor map is invalid: %w", err)
		}
	} else if command.AnchorMap != nil {
		return preparedDocumentProjection{}, errors.New("plaintext projection cannot carry a Markdown anchor map")
	}
	artifactType := documentProjectionDomainType(command.ArtifactType)
	digest := sha256.Sum256(command.ProjectionBytes)
	prepared := preparedDocumentProjection{
		reservation: ReserveDerivedArtifactCommand{
			DocumentVersionID: command.DocumentVersionID, ExpectedDocumentVersion: command.ExpectedDocumentVersion,
			StoreDerivedRightsDecisionID: command.StoreDerivedRightsDecisionID,
			RetainRightsDecisionID:       command.RetainRightsDecisionID,
			ArtifactType:                 string(artifactType), TransformerProfileSHA256: command.TransformerProfileSHA256,
			MIMEType: artifactType.MIMEType(), SHA256: fmt.Sprintf("%x", digest), SizeBytes: int64(len(command.ProjectionBytes)),
		},
		format: documentProjectionKnowledgeFormat(command.ArtifactType),
	}
	if command.AnchorMap != nil {
		prepared.reservation.AnchorMap = &DerivedArtifactAnchorMapDTO{
			NormalizationVersion: command.AnchorMap.NormalizationVersion, AnchorMapProfileVersion: command.AnchorMap.AnchorMapProfileVersion,
			PlaintextSHA256: command.AnchorMap.PlaintextSHA256, MarkdownSHA256: command.AnchorMap.MarkdownSHA256,
			AnchorMapSHA256: command.AnchorMap.AnchorMapSHA256,
		}
		prepared.anchorBlocks = cloneDocumentAnchorBlocks(command.AnchorMap.Blocks)
	}
	if err := ValidateReserveDerivedArtifactCommand(prepared.reservation); err != nil {
		return preparedDocumentProjection{}, err
	}
	return prepared, nil
}

func validateDerivedArtifactReservation(result ReserveDerivedArtifactResult, command ReserveDerivedArtifactCommand) error {
	documentVersion, versionErr := documentVersionDomainFromDTO(result.DocumentVersion)
	artifact, artifactErr := derivedArtifactDomainFromDTO(result.Artifact)
	if versionErr != nil || artifactErr != nil || result.DocumentID <= 0 || documentVersion.ID != command.DocumentVersionID ||
		documentVersion.DocumentID != result.DocumentID || artifact.DocumentVersionID != command.DocumentVersionID ||
		artifact.StoreDerivedRightsDecisionID != command.StoreDerivedRightsDecisionID ||
		artifact.RetainRightsDecisionID != command.RetainRightsDecisionID {
		return fmt.Errorf("%w: reserved derived artifact identity changed", sharedrepository.ErrConflict)
	}
	commandArtifactType := ingestiondomain.DerivedArtifactType(command.ArtifactType)
	if artifact.ArtifactType != commandArtifactType || artifact.TransformerProfileSHA256 != command.TransformerProfileSHA256 ||
		artifact.MIMEType != command.MIMEType || artifact.SHA256 != command.SHA256 || artifact.SizeBytes != command.SizeBytes ||
		result.VaultRelativePath != documentProjectionRelativePath(result.DocumentID, command.DocumentVersionID, command.ArtifactType, command.TransformerProfileSHA256) {
		return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ErrDerivedArtifactContentConflict)
	}
	if !sameDerivedArtifactAnchorMap(result.Artifact.AnchorMap, command.AnchorMap) {
		return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ErrDerivedArtifactContentConflict)
	}
	if artifact.LifecycleState != ingestiondomain.DerivedArtifactPending && artifact.LifecycleState != ingestiondomain.DerivedArtifactAvailable {
		return fmt.Errorf("%w: reserved derived artifact lifecycle is not publishable", sharedrepository.ErrConflict)
	}
	if documentVersion.Version != command.ExpectedDocumentVersion && artifact.LifecycleState != ingestiondomain.DerivedArtifactAvailable {
		return fmt.Errorf("%w: document version changed before projection reserve", sharedrepository.ErrConflict)
	}
	return nil
}

func publishProjectionCommand(reservation ReserveDerivedArtifactResult, prepared preparedDocumentProjection, content []byte) knowledgeapplication.PublishProjectionCommand {
	return knowledgeapplication.PublishProjectionCommand{
		DocumentID: reservation.DocumentID, DocumentVersionID: prepared.reservation.DocumentVersionID,
		Format: prepared.format, TransformerProfileSHA256: prepared.reservation.TransformerProfileSHA256,
		Content: append([]byte(nil), content...), SHA256: prepared.reservation.SHA256,
	}
}

func projectionReceiptFromPublishResult(result knowledgeapplication.PublishProjectionResult, reservation ReserveDerivedArtifactResult, command ReserveDerivedArtifactCommand) (ProjectionReceiptDTO, error) {
	expectedFormat := documentProjectionKnowledgeFormat(command.ArtifactType)
	if result.DocumentID != reservation.DocumentID || result.DocumentVersionID != command.DocumentVersionID ||
		result.Format != expectedFormat || result.TransformerProfileSHA256 != command.TransformerProfileSHA256 ||
		result.RelativePath != reservation.VaultRelativePath || result.MIMEType != command.MIMEType ||
		result.SHA256 != command.SHA256 || result.SizeBytes != command.SizeBytes {
		return ProjectionReceiptDTO{}, fmt.Errorf("%w: projection receipt does not match reservation", sharedrepository.ErrConflict)
	}
	return ProjectionReceiptDTO{
		DocumentID: result.DocumentID, DocumentVersionID: result.DocumentVersionID,
		ArtifactType: command.ArtifactType, TransformerProfileSHA256: result.TransformerProfileSHA256,
		VaultRelativePath: result.RelativePath, MIMEType: result.MIMEType, SHA256: result.SHA256, SizeBytes: result.SizeBytes,
	}, nil
}

func validateCommittedDerivedArtifact(committed CommitDerivedArtifactResult, reservation ReserveDerivedArtifactResult, command ReserveDerivedArtifactCommand) error {
	documentVersion, versionErr := documentVersionDomainFromDTO(committed.DocumentVersion)
	artifact, artifactErr := derivedArtifactDomainFromDTO(committed.Artifact)
	if versionErr != nil || artifactErr != nil || committed.DocumentID != reservation.DocumentID ||
		documentVersion.ID != command.DocumentVersionID || documentVersion.DocumentID != reservation.DocumentID ||
		artifact.ID != reservation.Artifact.ID || artifact.LifecycleState != ingestiondomain.DerivedArtifactAvailable ||
		artifact.DocumentVersionID != command.DocumentVersionID {
		return fmt.Errorf("%w: committed derived artifact identity changed", sharedrepository.ErrConflict)
	}
	if artifact.ArtifactType != ingestiondomain.DerivedArtifactType(command.ArtifactType) ||
		artifact.TransformerProfileSHA256 != command.TransformerProfileSHA256 || artifact.SHA256 != command.SHA256 ||
		artifact.SizeBytes != command.SizeBytes || artifact.MIMEType != command.MIMEType {
		return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ErrDerivedArtifactContentConflict)
	}
	if !sameDerivedArtifactAnchorMap(committed.Artifact.AnchorMap, command.AnchorMap) {
		return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ErrDerivedArtifactContentConflict)
	}
	return nil
}

func validateDerivedArtifactAnchorMapDTO(value *DerivedArtifactAnchorMapDTO, markdownSHA256 string) error {
	if value == nil || value.NormalizationVersion != CanonicalDocumentTextNormalizationVersion ||
		value.AnchorMapProfileVersion != CanonicalDocumentAnchorMapProfileVersion ||
		!validLowerHexSHA256(value.PlaintextSHA256) || !validLowerHexSHA256(value.MarkdownSHA256) ||
		!validLowerHexSHA256(value.AnchorMapSHA256) || value.MarkdownSHA256 != markdownSHA256 {
		return errors.New("derived artifact anchor map identity is invalid")
	}
	return nil
}

func sameDerivedArtifactAnchorMap(left, right *DerivedArtifactAnchorMapDTO) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.NormalizationVersion == right.NormalizationVersion && left.AnchorMapProfileVersion == right.AnchorMapProfileVersion &&
		left.PlaintextSHA256 == right.PlaintextSHA256 && left.MarkdownSHA256 == right.MarkdownSHA256 && left.AnchorMapSHA256 == right.AnchorMapSHA256
}

func cloneDocumentAnchorBlocks(values []DocumentAnchorBlockDTO) []DocumentAnchorBlockDTO {
	return append([]DocumentAnchorBlockDTO(nil), values...)
}

func (service *DocumentProjectionService) advanceDocumentVersion(ctx context.Context, current ingestiondomain.DocumentVersion, displayDecisionID *int64) (ingestiondomain.DocumentVersion, error) {
	var err error
	switch current.LifecycleState {
	case ingestiondomain.DocumentPolicyPending, ingestiondomain.DocumentPolicyBlocked, ingestiondomain.DocumentDerivedFailed:
		current, err = service.transitionDocumentVersion(ctx, current, ingestiondomain.DocumentDerivedPending, nil)
		if err != nil {
			return ingestiondomain.DocumentVersion{}, err
		}
	case ingestiondomain.DocumentDerivedPending, ingestiondomain.DocumentDerivedAvailable, ingestiondomain.DocumentReadable:
	default:
		return ingestiondomain.DocumentVersion{}, fmt.Errorf("%w: document lifecycle is not projectable", sharedrepository.ErrConflict)
	}
	if current.LifecycleState == ingestiondomain.DocumentDerivedPending {
		current, err = service.transitionDocumentVersion(ctx, current, ingestiondomain.DocumentDerivedAvailable, nil)
		if err != nil {
			return ingestiondomain.DocumentVersion{}, err
		}
	}
	if displayDecisionID != nil && current.LifecycleState == ingestiondomain.DocumentDerivedAvailable {
		current, err = service.transitionDocumentVersion(ctx, current, ingestiondomain.DocumentReadable, displayDecisionID)
		if err != nil {
			return ingestiondomain.DocumentVersion{}, err
		}
	}
	return current, nil
}

func (service *DocumentProjectionService) transitionDocumentVersion(ctx context.Context, current ingestiondomain.DocumentVersion, to ingestiondomain.DocumentLifecycleState, displayDecisionID *int64) (ingestiondomain.DocumentVersion, error) {
	result, err := service.documentVersions.TransitionDocumentVersion(ctx, TransitionDocumentVersionCommand{
		DocumentVersionID: current.ID, ExpectedVersion: current.Version, To: string(to),
		DisplayPrivateRightsDecisionID: displayDecisionID,
	})
	if err != nil {
		return ingestiondomain.DocumentVersion{}, err
	}
	updated, mapErr := documentVersionDomainFromDTO(result.DocumentVersion)
	if mapErr != nil {
		return ingestiondomain.DocumentVersion{}, fmt.Errorf("%w: lifecycle returned invalid document version", sharedrepository.ErrConflict)
	}
	return updated, nil
}

func (service *DocumentProjectionService) failProjection(ctx context.Context, reservation ReserveDerivedArtifactResult, failureCode string, cause error) error {
	artifact, artifactErr := derivedArtifactDomainFromDTO(reservation.Artifact)
	documentVersion, versionErr := documentVersionDomainFromDTO(reservation.DocumentVersion)
	if artifactErr != nil || versionErr != nil {
		return fmt.Errorf("document projection failed; %w: invalid reservation DTO", sharedrepository.ErrConflict)
	}
	if artifact.LifecycleState == ingestiondomain.DerivedArtifactPending {
		if _, err := service.repository.MarkFailed(ctx, MarkDerivedArtifactFailedCommand{
			ArtifactID: reservation.Artifact.ID, FailureCode: failureCode,
		}); err != nil {
			return fmt.Errorf("document projection failed; mark derived artifact failed: %w", err)
		}
	}
	if documentVersion.LifecycleState != ingestiondomain.DocumentDerivedAvailable &&
		documentVersion.LifecycleState != ingestiondomain.DocumentReadable &&
		documentVersion.LifecycleState != ingestiondomain.DocumentDerivedFailed {
		current := documentVersion
		var err error
		if current.LifecycleState != ingestiondomain.DocumentDerivedPending {
			current, err = service.transitionDocumentVersion(ctx, current, ingestiondomain.DocumentDerivedPending, nil)
			if err != nil {
				return fmt.Errorf("document projection failed; prepare failure lifecycle: %w", err)
			}
		}
		if _, err := service.transitionDocumentVersion(ctx, current, ingestiondomain.DocumentDerivedFailed, nil); err != nil {
			return fmt.Errorf("document projection failed; persist failure lifecycle: %w", err)
		}
	}
	return fmt.Errorf("document projection failed: %w", cause)
}

func (service *DocumentProjectionService) quarantineContentConflict(ctx context.Context, reservation ReserveDerivedArtifactResult) error {
	documentVersion, versionErr := documentVersionDomainFromDTO(reservation.DocumentVersion)
	if versionErr != nil {
		return fmt.Errorf("%w: conflicting reservation document version is invalid", sharedrepository.ErrConflict)
	}
	if _, err := service.repository.Quarantine(ctx, QuarantineDerivedArtifactCommand{ArtifactID: reservation.Artifact.ID}); err != nil {
		return fmt.Errorf("%w: quarantine conflicting derived artifact: %w", sharedrepository.ErrConflict, err)
	}
	if documentVersion.LifecycleState != ingestiondomain.DocumentQuarantined && documentVersion.LifecycleState != ingestiondomain.DocumentTombstoned {
		if _, err := service.transitionDocumentVersion(ctx, documentVersion, ingestiondomain.DocumentQuarantined, nil); err != nil {
			return fmt.Errorf("%w: quarantine conflicting document version: %w", sharedrepository.ErrConflict, err)
		}
	}
	return fmt.Errorf("%w: immutable document projection content conflict", sharedrepository.ErrConflict)
}

func projectionContentConflict(err error) bool {
	return errors.Is(err, knowledgeapplication.ErrProjectionConflict) || errors.Is(err, knowledgeapplication.ErrProjectionIntegrity)
}

func projectDocumentResult(artifact ingestiondomain.DerivedArtifact, documentVersion ingestiondomain.DocumentVersion) ProjectDocumentResult {
	return ProjectDocumentResult{
		Artifact:        derivedArtifactDTOFromDomain(artifact),
		DocumentVersion: documentVersionDTOFromDomain(documentVersion),
	}
}

func documentProjectionRelativePath(documentID, documentVersionID int64, artifactType string, profileSHA string) string {
	extension := "txt"
	if artifactType == DerivedArtifactMarkdown {
		extension = "md"
	}
	return path.Join("documents", fmt.Sprintf("%d", documentID), fmt.Sprintf("%d", documentVersionID), artifactType, profileSHA+"."+extension)
}

func documentProjectionDomainType(format string) ingestiondomain.DerivedArtifactType {
	if format == DocumentProjectionMarkdown {
		return ingestiondomain.DerivedArtifactMarkdown
	}
	if format == DocumentProjectionPlaintext {
		return ingestiondomain.DerivedArtifactPlaintext
	}
	return ""
}

func documentProjectionApplicationType(artifactType ingestiondomain.DerivedArtifactType) string {
	if artifactType == ingestiondomain.DerivedArtifactMarkdown {
		return DocumentProjectionMarkdown
	}
	if artifactType == ingestiondomain.DerivedArtifactPlaintext {
		return DocumentProjectionPlaintext
	}
	return ""
}

func documentProjectionKnowledgeFormat(format string) knowledgeapplication.ProjectionFormat {
	if format == DocumentProjectionMarkdown {
		return knowledgeapplication.ProjectionFormatMarkdown
	}
	if format == DocumentProjectionPlaintext {
		return knowledgeapplication.ProjectionFormatPlaintext
	}
	return ""
}
