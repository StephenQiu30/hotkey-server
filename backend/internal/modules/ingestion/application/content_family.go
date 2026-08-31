package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

var ErrInvalidContentFamilyContract = errors.New("content family contract is invalid")

type ContentFingerprintDTO struct {
	ProfileVersion          string
	NormalizedContentSHA256 string
	SimHashHex              string
	MinHash                 []uint64
}

type ContentFamilyCandidateDTO struct {
	FamilyID              int64
	FamilyVersion         int64
	RootDocumentVersionID int64
	Fingerprint           ContentFingerprintDTO
}

type FindContentFamilyCandidatesQuery struct {
	DocumentVersionID int64
	Fingerprint       ContentFingerprintDTO
	Limit             int
	DecisionAt        time.Time
}

type CommitContentFamilyDecisionCommand struct {
	SourceConnectionID           int64
	DocumentVersionID            int64
	DerivedArtifactID            int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	RetentionUntil               time.Time
	DecisionAt                   time.Time
	Fingerprint                  ContentFingerprintDTO
	Action                       string
	FamilyID                     int64
	ExpectedFamilyVersion        int64
	RootDocumentVersionID        int64
	Relation                     string
	HammingDistance              int
	MinHashSimilarity            float64
	DecisionProfileVersion       string
	ReasonCodes                  []string
	IdempotencyKey               string
	CommandFingerprint           string
}

type ContentFamilyDecisionDTO struct {
	DecisionID             int64
	FamilyID               int64
	FamilyVersion          int64
	DocumentVersionID      int64
	RootDocumentVersionID  int64
	Action                 string
	Relation               string
	HammingDistance        int
	MinHashSimilarity      float64
	DecisionProfileVersion string
	ReasonCodes            []string
	Reused                 bool
}

type AssignDocumentContentFamilyCommand struct {
	SourceConnectionID           int64
	DocumentVersionID            int64
	DerivedArtifactID            int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	RetentionUntil               time.Time
	DecisionAt                   time.Time
	CanonicalPlaintext           string
	FingerprintProfile           string
	DecisionProfileVersion       string
	HardConflict                 bool
}

type AssignDocumentContentFamilyResult struct{ Decision ContentFamilyDecisionDTO }

type ContentFamilyRepository interface {
	FindContentFamilyCandidates(context.Context, FindContentFamilyCandidatesQuery) ([]ContentFamilyCandidateDTO, error)
	CommitContentFamilyDecision(context.Context, CommitContentFamilyDecisionCommand) (ContentFamilyDecisionDTO, error)
}

type ContentFamilyQualityProfileReader interface {
	IsDecisionQualityProfileActive(context.Context, string, string) (bool, error)
}

type ContentFamilyService struct {
	repository      ContentFamilyRepository
	qualityProfiles ContentFamilyQualityProfileReader
}

func NewContentFamilyService(repository ContentFamilyRepository) (*ContentFamilyService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidContentFamilyContract)
	}
	return &ContentFamilyService{repository: repository}, nil
}

func NewContentFamilyServiceWithQualityProfiles(repository ContentFamilyRepository, profiles ContentFamilyQualityProfileReader) (*ContentFamilyService, error) {
	if profiles == nil {
		return nil, fmt.Errorf("%w: quality profile reader is required", ErrInvalidContentFamilyContract)
	}
	service, err := NewContentFamilyService(repository)
	if err != nil {
		return nil, err
	}
	service.qualityProfiles = profiles
	return service, nil
}

func (service *ContentFamilyService) Assign(ctx context.Context, command AssignDocumentContentFamilyCommand) (AssignDocumentContentFamilyResult, error) {
	if service == nil || service.repository == nil || command.SourceConnectionID <= 0 || command.DocumentVersionID <= 0 ||
		command.DerivedArtifactID <= 0 || command.StoreDerivedRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		command.DecisionAt.IsZero() || !command.RetentionUntil.After(command.DecisionAt) ||
		strings.TrimSpace(command.CanonicalPlaintext) == "" || strings.TrimSpace(command.FingerprintProfile) == "" ||
		strings.TrimSpace(command.DecisionProfileVersion) == "" {
		return AssignDocumentContentFamilyResult{}, ErrInvalidContentFamilyContract
	}
	fingerprint, err := ingestiondomain.BuildContentFingerprint(command.CanonicalPlaintext, command.FingerprintProfile)
	if err != nil {
		return AssignDocumentContentFamilyResult{}, fmt.Errorf("%w: %w", ErrInvalidContentFamilyContract, err)
	}
	fingerprintDTO := contentFingerprintDTO(fingerprint)
	candidateDTOs, err := service.repository.FindContentFamilyCandidates(ctx, FindContentFamilyCandidatesQuery{
		DocumentVersionID: command.DocumentVersionID, Fingerprint: fingerprintDTO, Limit: 50, DecisionAt: command.DecisionAt.UTC(),
	})
	if err != nil {
		return AssignDocumentContentFamilyResult{}, fmt.Errorf("find content family candidates: %w", err)
	}
	candidates := make([]ingestiondomain.ContentFamilyCandidate, len(candidateDTOs))
	for index, candidateDTO := range candidateDTOs {
		candidate, mapErr := contentFamilyCandidateFromDTO(candidateDTO)
		if mapErr != nil {
			return AssignDocumentContentFamilyResult{}, fmt.Errorf("%w: invalid candidate receipt", ErrInvalidContentFamilyContract)
		}
		candidates[index] = candidate
	}
	decision, err := ingestiondomain.DecideContentFamily(ingestiondomain.ContentFamilyDecisionInput{
		DocumentVersionID: command.DocumentVersionID, Fingerprint: fingerprint, Candidates: candidates,
		DecisionProfileVersion: command.DecisionProfileVersion, HardConflict: command.HardConflict,
	})
	if err != nil {
		return AssignDocumentContentFamilyResult{}, fmt.Errorf("%w: %w", ErrInvalidContentFamilyContract, err)
	}
	if decision.Action == ingestiondomain.ContentFamilyActionJoin && service.qualityProfiles != nil {
		active, readErr := service.qualityProfiles.IsDecisionQualityProfileActive(ctx, "content_family", command.DecisionProfileVersion)
		if readErr != nil || !active {
			decision.Action = ingestiondomain.ContentFamilyActionReview
			decision.ReasonCodes = append(decision.ReasonCodes, "quality_profile_not_active")
		}
	}
	mutation := CommitContentFamilyDecisionCommand{
		SourceConnectionID: command.SourceConnectionID, DocumentVersionID: command.DocumentVersionID,
		DerivedArtifactID: command.DerivedArtifactID, StoreDerivedRightsDecisionID: command.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID: command.RetainRightsDecisionID, RetentionUntil: command.RetentionUntil.UTC(), DecisionAt: command.DecisionAt.UTC(),
		Fingerprint: fingerprintDTO,
		Action:      string(decision.Action), FamilyID: decision.FamilyID, ExpectedFamilyVersion: decision.FamilyVersion,
		RootDocumentVersionID: decision.RootDocumentVersionID, Relation: string(decision.Relation),
		HammingDistance: decision.HammingDistance, MinHashSimilarity: decision.MinHashSimilarity,
		DecisionProfileVersion: decision.DecisionProfileVersion, ReasonCodes: append([]string(nil), decision.ReasonCodes...),
	}
	mutation.IdempotencyKey, mutation.CommandFingerprint = contentFamilyMutationIdentity(mutation, command.HardConflict)
	persisted, err := service.repository.CommitContentFamilyDecision(ctx, mutation)
	if err != nil {
		return AssignDocumentContentFamilyResult{}, fmt.Errorf("commit content family decision: %w", err)
	}
	if persisted.Reused {
		if !validReusedContentFamilyDecisionReceipt(persisted, command) {
			return AssignDocumentContentFamilyResult{}, fmt.Errorf("%w: replayed content family receipt changed", ErrInvalidContentFamilyContract)
		}
	} else if !contentFamilyDecisionReceiptMatches(persisted, command.DocumentVersionID, decision) {
		return AssignDocumentContentFamilyResult{}, fmt.Errorf("%w: content family receipt changed", ErrInvalidContentFamilyContract)
	}
	return AssignDocumentContentFamilyResult{Decision: persisted}, nil
}

func validReusedContentFamilyDecisionReceipt(value ContentFamilyDecisionDTO, command AssignDocumentContentFamilyCommand) bool {
	return value.DecisionID > 0 && value.FamilyID > 0 && value.FamilyVersion > 0 &&
		value.DocumentVersionID == command.DocumentVersionID && value.RootDocumentVersionID > 0 &&
		validContentFamilyDecisionAction(value.Action) && validContentFamilyDecisionRelation(value.Relation) &&
		value.HammingDistance >= 0 && value.HammingDistance <= 64 && value.MinHashSimilarity >= 0 && value.MinHashSimilarity <= 1 &&
		value.DecisionProfileVersion == command.DecisionProfileVersion && len(value.ReasonCodes) > 0
}

func validContentFamilyDecisionAction(value string) bool {
	return value == "create" || value == "join" || value == "review"
}

func validContentFamilyDecisionRelation(value string) bool {
	switch value {
	case "exact_copy", "near_duplicate", "syndicated_from", "translation_of", "revision_of", "unrelated":
		return true
	default:
		return false
	}
}

func contentFamilyMutationIdentity(command CommitContentFamilyDecisionCommand, hardConflict bool) (string, string) {
	idempotencyDigest := sha256.Sum256([]byte(fmt.Sprintf("content-family:%d:%s:%s", command.DocumentVersionID,
		command.Fingerprint.ProfileVersion, command.Fingerprint.NormalizedContentSHA256)))
	fingerprintDigest := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%d|%d|%d|%s|%s|%s|%s|%t",
		command.SourceConnectionID, command.DocumentVersionID, command.DerivedArtifactID,
		command.StoreDerivedRightsDecisionID, command.RetainRightsDecisionID, command.RetentionUntil.UTC().Format(time.RFC3339Nano),
		command.Fingerprint.ProfileVersion, command.Fingerprint.NormalizedContentSHA256, command.DecisionProfileVersion, hardConflict)))
	return "content-family-" + hex.EncodeToString(idempotencyDigest[:16]), hex.EncodeToString(fingerprintDigest[:])
}

func contentFingerprintDTO(value ingestiondomain.ContentFingerprint) ContentFingerprintDTO {
	return ContentFingerprintDTO{ProfileVersion: value.ProfileVersion, NormalizedContentSHA256: value.NormalizedContentSHA256,
		SimHashHex: value.SimHashHex, MinHash: append([]uint64(nil), value.MinHash...)}
}

func contentFingerprintFromDTO(value ContentFingerprintDTO) (ingestiondomain.ContentFingerprint, error) {
	result := ingestiondomain.ContentFingerprint{ProfileVersion: value.ProfileVersion, NormalizedContentSHA256: value.NormalizedContentSHA256,
		SimHashHex: value.SimHashHex, MinHash: append([]uint64(nil), value.MinHash...)}
	if err := result.Validate(); err != nil {
		return ingestiondomain.ContentFingerprint{}, err
	}
	return result, nil
}

func contentFamilyCandidateFromDTO(value ContentFamilyCandidateDTO) (ingestiondomain.ContentFamilyCandidate, error) {
	fingerprint, err := contentFingerprintFromDTO(value.Fingerprint)
	if err != nil {
		return ingestiondomain.ContentFamilyCandidate{}, err
	}
	return ingestiondomain.ContentFamilyCandidate{FamilyID: value.FamilyID, FamilyVersion: value.FamilyVersion,
		RootDocumentVersionID: value.RootDocumentVersionID, Fingerprint: fingerprint}, nil
}

func contentFamilyDecisionReceiptMatches(value ContentFamilyDecisionDTO, documentVersionID int64, decision ingestiondomain.ContentFamilyDecision) bool {
	return value.DecisionID > 0 && value.FamilyID > 0 && value.FamilyVersion > 0 && value.DocumentVersionID == documentVersionID &&
		value.RootDocumentVersionID > 0 && value.Action == string(decision.Action) && value.Relation == string(decision.Relation) &&
		value.HammingDistance == decision.HammingDistance && value.MinHashSimilarity == decision.MinHashSimilarity &&
		value.DecisionProfileVersion == decision.DecisionProfileVersion && stringSlicesEqual(value.ReasonCodes, decision.ReasonCodes)
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
