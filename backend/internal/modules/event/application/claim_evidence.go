package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	"golang.org/x/text/unicode/norm"
)

const (
	CanonicalClaimExtractionSchemaVersion  = "atomic-claim-evidence-v2"
	CanonicalEvidenceStateAlgorithmVersion = "evidence-state-lineage-v2"
)

var ErrInvalidClaimEvidenceContract = errors.New("claim evidence contract is invalid")

type ClaimQualifierDTO struct {
	Key   string
	Value string
}

type RecordClaimEvidenceCommand struct {
	MicroEventID            int64
	ExpectedEventVersion    int64
	DocumentVersionID       int64
	TextQuoteSelectorID     int64
	Subject                 string
	Predicate               string
	Object                  string
	Qualifiers              []ClaimQualifierDTO
	Relation                string
	ModelRunID              *int64
	ModelRelationScore      *float64
	ExtractionSchemaVersion string
	Origin                  string
	ActorUserID             *int64
	IdempotencyKey          string
	DecisionAt              time.Time
}

type ClaimEvidenceTargetQuery struct {
	MicroEventID        int64
	DocumentVersionID   int64
	TextQuoteSelectorID int64
	DecisionAt          time.Time
}

type ClaimEvidenceTargetDTO struct {
	MicroEventID           int64
	MicroEventVersion      int64
	DocumentVersionID      int64
	TextQuoteSelectorID    int64
	ContentFamilyID        int64
	LineageRootID          int64
	QuoteSHA256            string
	PlaintextSHA256        string
	SelectorVersion        string
	SelectorRetentionUntil time.Time
	CurrentlyCitable       bool
	SourceRecordURL        *string
	CanonicalURL           *string
	PublisherPartyID       *int64
	PublisherName          *string
	ContentOriginPartyID   *int64
	ContentOriginName      *string
	PublishedAt            *time.Time
	CapturedAt             time.Time
}

type CommitClaimEvidenceCommand struct {
	Target                  ClaimEvidenceTargetDTO
	ClaimHash               string
	Subject                 string
	Predicate               string
	Object                  string
	Qualifiers              []ClaimQualifierDTO
	Relation                string
	ModelRunID              *int64
	ModelRelationScore      *float64
	ExtractionSchemaVersion string
	Origin                  string
	ActorUserID             *int64
	IdempotencyKey          string
	CommandFingerprint      string
	DecisionAt              time.Time
}

type ClaimDTO struct {
	ID, Version, MicroEventID, MicroEventVersion int64
	ClaimHash, Subject, Predicate, Object        string
	Qualifiers                                   []ClaimQualifierDTO
	CreatedAt                                    time.Time
}

type ClaimEvidenceVersionDTO struct {
	ID, Version, ClaimID, DocumentVersionID, TextQuoteSelectorID int64
	ContentFamilyID, LineageRootID                               int64
	Relation, ExtractionSchemaVersion, Origin                    string
	ModelRunID, ActorUserID                                      *int64
	ModelRelationScore                                           *float64
	QuoteSHA256, PlaintextSHA256, SelectorVersion                string
	SourceRecordURL, CanonicalURL                                *string
	PublisherPartyID, ContentOriginPartyID                       *int64
	PublisherName, ContentOriginName                             *string
	PublishedAt                                                  *time.Time
	CapturedAt, RetentionUntil                                   time.Time
	CreatedAt                                                    time.Time
}

type RecordClaimEvidenceResult struct {
	Claim    ClaimDTO
	Evidence ClaimEvidenceVersionDTO
	Created  bool
}

type EvidenceStateTargetQuery struct {
	MicroEventID     int64
	EventVersion     int64
	AlgorithmVersion string
	CalculatedAt     time.Time
}

type EvidenceStateItemDTO struct {
	ClaimEvidenceVersionID int64
	LineageRootID          int64
	Relation               string
	Citable                bool
}

type EvidenceStateTargetDTO struct {
	MicroEventID, EventVersion, ProfileID int64
	AlgorithmVersion                      string
	Items                                 []EvidenceStateItemDTO
}

type CommitEvidenceStateSnapshotCommand struct {
	MicroEventID, EventVersion, ProfileID int64
	AlgorithmVersion, EvidenceSetHash     string
	State                                 string
	IndependentOriginCount                int
	ReasonCodes                           []string
	ClaimEvidenceVersionIDs               []int64
	CalculatedAt                          time.Time
}

type EvidenceStateSnapshotDTO struct {
	ID, Version, MicroEventID, EventVersion, ProfileID int64
	AlgorithmVersion, EvidenceSetHash, State           string
	IndependentOriginCount                             int
	ReasonCodes                                        []string
	ClaimEvidenceVersionIDs                            []int64
	CalculatedAt                                       time.Time
	Created                                            bool
}

type CalculateEvidenceStateCommand struct {
	MicroEventID, ExpectedEventVersion int64
	AlgorithmVersion                   string
	CalculatedAt                       time.Time
}

type CalculateEvidenceStateResult struct{ Snapshot EvidenceStateSnapshotDTO }

type CorrectClaimEvidenceCommand struct {
	OriginalClaimEvidenceVersionID int64
	ExpectedClaimVersion           int64
	ResultTextQuoteSelectorID      int64
	ResultRelation                 string
	ActorUserID                    int64
	ReasonCode                     string
	Note                           string
	IdempotencyKey                 string
	DecisionAt                     time.Time
}

type ClaimEvidenceCorrectionTargetQuery struct {
	OriginalClaimEvidenceVersionID int64
	ResultTextQuoteSelectorID      int64
	DecisionAt                     time.Time
}

type ClaimEvidenceCorrectionTargetDTO struct {
	Claim            ClaimDTO
	OriginalEvidence ClaimEvidenceVersionDTO
	ResultTarget     ClaimEvidenceTargetDTO
}

type CommitClaimEvidenceCorrectionCommand struct {
	Target             ClaimEvidenceCorrectionTargetDTO
	ResultRelation     string
	ActorUserID        int64
	ReasonCode         string
	Note               string
	IdempotencyKey     string
	CommandFingerprint string
	DecisionAt         time.Time
}

type ClaimEvidenceFeedbackDTO struct {
	ID, Version, ClaimID, OriginalClaimEvidenceVersionID, ResultClaimEvidenceVersionID int64
	TargetDocumentVersionID, OriginalTextQuoteSelectorID, ResultTextQuoteSelectorID    int64
	OriginalRelation, ResultRelation, ReasonCode, Note                                 string
	ActorUserID, ExpectedClaimVersion                                                  int64
	CreatedAt                                                                          time.Time
}

type CorrectClaimEvidenceResult struct {
	Evidence ClaimEvidenceVersionDTO
	Feedback ClaimEvidenceFeedbackDTO
	Created  bool
}

type ClaimEvidenceRepository interface {
	ReadClaimEvidenceTarget(context.Context, ClaimEvidenceTargetQuery) (ClaimEvidenceTargetDTO, error)
	CommitClaimEvidence(context.Context, CommitClaimEvidenceCommand) (RecordClaimEvidenceResult, error)
	ReadEvidenceStateTarget(context.Context, EvidenceStateTargetQuery) (EvidenceStateTargetDTO, error)
	CommitEvidenceStateSnapshot(context.Context, CommitEvidenceStateSnapshotCommand) (EvidenceStateSnapshotDTO, error)
	ReadClaimEvidenceCorrectionTarget(context.Context, ClaimEvidenceCorrectionTargetQuery) (ClaimEvidenceCorrectionTargetDTO, error)
	CommitClaimEvidenceCorrection(context.Context, CommitClaimEvidenceCorrectionCommand) (CorrectClaimEvidenceResult, error)
}

func (service *ClaimEvidenceService) Correct(ctx context.Context, command CorrectClaimEvidenceCommand) (CorrectClaimEvidenceResult, error) {
	command.ResultRelation, command.ReasonCode, command.Note = strings.TrimSpace(command.ResultRelation), strings.TrimSpace(command.ReasonCode), strings.TrimSpace(command.Note)
	command.IdempotencyKey, command.DecisionAt = strings.TrimSpace(command.IdempotencyKey), command.DecisionAt.UTC()
	if service == nil || service.repository == nil || command.OriginalClaimEvidenceVersionID <= 0 || command.ExpectedClaimVersion <= 0 ||
		command.ResultTextQuoteSelectorID <= 0 || !eventdomain.ClaimEvidenceRelation(command.ResultRelation).Valid() || command.ActorUserID <= 0 ||
		command.ReasonCode == "" || len(command.ReasonCode) > 64 || len(command.Note) > 1000 || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 96 || command.DecisionAt.IsZero() {
		return CorrectClaimEvidenceResult{}, ErrInvalidClaimEvidenceContract
	}
	target, err := service.repository.ReadClaimEvidenceCorrectionTarget(ctx, ClaimEvidenceCorrectionTargetQuery{
		OriginalClaimEvidenceVersionID: command.OriginalClaimEvidenceVersionID,
		ResultTextQuoteSelectorID:      command.ResultTextQuoteSelectorID, DecisionAt: command.DecisionAt})
	if err != nil {
		return CorrectClaimEvidenceResult{}, fmt.Errorf("read claim evidence correction target: %w", err)
	}
	if target.Claim.ID <= 0 || target.Claim.Version != command.ExpectedClaimVersion ||
		target.OriginalEvidence.ID != command.OriginalClaimEvidenceVersionID || target.OriginalEvidence.ClaimID != target.Claim.ID ||
		target.ResultTarget.MicroEventID != target.Claim.MicroEventID || target.ResultTarget.MicroEventVersion != target.Claim.MicroEventVersion ||
		target.ResultTarget.DocumentVersionID != target.OriginalEvidence.DocumentVersionID ||
		target.ResultTarget.TextQuoteSelectorID != command.ResultTextQuoteSelectorID || !target.ResultTarget.CurrentlyCitable ||
		target.ResultTarget.DecisionAtAfterRetention(command.DecisionAt) ||
		(target.OriginalEvidence.TextQuoteSelectorID == command.ResultTextQuoteSelectorID && target.OriginalEvidence.Relation == command.ResultRelation) {
		return CorrectClaimEvidenceResult{}, ErrInvalidClaimEvidenceContract
	}
	fingerprintPayload, _ := json.Marshal(struct {
		Command CorrectClaimEvidenceCommand
		Target  ClaimEvidenceCorrectionTargetDTO
	}{command, target})
	digest := sha256.Sum256(fingerprintPayload)
	result, err := service.repository.CommitClaimEvidenceCorrection(ctx, CommitClaimEvidenceCorrectionCommand{
		Target: target, ResultRelation: command.ResultRelation, ActorUserID: command.ActorUserID,
		ReasonCode: command.ReasonCode, Note: command.Note, IdempotencyKey: command.IdempotencyKey,
		CommandFingerprint: hex.EncodeToString(digest[:]), DecisionAt: command.DecisionAt,
	})
	if err != nil {
		return CorrectClaimEvidenceResult{}, fmt.Errorf("commit claim evidence correction: %w", err)
	}
	if result.Evidence.ID <= 0 || result.Feedback.ID <= 0 || result.Feedback.ClaimID != target.Claim.ID ||
		result.Feedback.OriginalClaimEvidenceVersionID != target.OriginalEvidence.ID || result.Feedback.ResultClaimEvidenceVersionID != result.Evidence.ID ||
		result.Feedback.ResultTextQuoteSelectorID != command.ResultTextQuoteSelectorID || result.Feedback.ResultRelation != command.ResultRelation ||
		result.Feedback.ActorUserID != command.ActorUserID || result.Feedback.ExpectedClaimVersion != command.ExpectedClaimVersion {
		return CorrectClaimEvidenceResult{}, ErrInvalidClaimEvidenceContract
	}
	return result, nil
}

type ClaimEvidenceService struct{ repository ClaimEvidenceRepository }

func NewClaimEvidenceService(repository ClaimEvidenceRepository) (*ClaimEvidenceService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidClaimEvidenceContract)
	}
	return &ClaimEvidenceService{repository: repository}, nil
}

func (service *ClaimEvidenceService) Record(ctx context.Context, command RecordClaimEvidenceCommand) (RecordClaimEvidenceResult, error) {
	canonical, err := canonicalClaimEvidenceCommand(command)
	if err != nil || service == nil || service.repository == nil {
		return RecordClaimEvidenceResult{}, ErrInvalidClaimEvidenceContract
	}
	target, err := service.repository.ReadClaimEvidenceTarget(ctx, ClaimEvidenceTargetQuery{
		MicroEventID: canonical.MicroEventID, DocumentVersionID: canonical.DocumentVersionID,
		TextQuoteSelectorID: canonical.TextQuoteSelectorID, DecisionAt: canonical.DecisionAt,
	})
	if err != nil {
		return RecordClaimEvidenceResult{}, fmt.Errorf("read claim evidence target: %w", err)
	}
	if target.MicroEventID != canonical.MicroEventID || target.MicroEventVersion != canonical.ExpectedEventVersion ||
		target.DocumentVersionID != canonical.DocumentVersionID || target.TextQuoteSelectorID != canonical.TextQuoteSelectorID ||
		target.ContentFamilyID <= 0 || target.LineageRootID <= 0 || !target.CurrentlyCitable ||
		target.DecisionAtAfterRetention(canonical.DecisionAt) || !lowerHexDigest(target.QuoteSHA256) || !lowerHexDigest(target.PlaintextSHA256) {
		return RecordClaimEvidenceResult{}, ErrInvalidClaimEvidenceContract
	}
	claimHash := claimEvidenceClaimHash(canonical.Subject, canonical.Predicate, canonical.Object, canonical.Qualifiers)
	fingerprint := claimEvidenceCommandFingerprint(canonical, target, claimHash)
	result, err := service.repository.CommitClaimEvidence(ctx, CommitClaimEvidenceCommand{
		Target: target, ClaimHash: claimHash, Subject: canonical.Subject, Predicate: canonical.Predicate,
		Object: canonical.Object, Qualifiers: canonical.Qualifiers, Relation: canonical.Relation,
		ModelRunID: canonical.ModelRunID, ModelRelationScore: canonical.ModelRelationScore,
		ExtractionSchemaVersion: canonical.ExtractionSchemaVersion, Origin: canonical.Origin,
		ActorUserID: canonical.ActorUserID, IdempotencyKey: canonical.IdempotencyKey,
		CommandFingerprint: fingerprint, DecisionAt: canonical.DecisionAt,
	})
	if err != nil {
		return RecordClaimEvidenceResult{}, fmt.Errorf("commit claim evidence: %w", err)
	}
	if !claimEvidenceReceiptMatches(result, canonical, target, claimHash) {
		return RecordClaimEvidenceResult{}, ErrInvalidClaimEvidenceContract
	}
	return result, nil
}

func (target ClaimEvidenceTargetDTO) DecisionAtAfterRetention(at time.Time) bool {
	return target.SelectorRetentionUntil.IsZero() || !target.SelectorRetentionUntil.After(at)
}

func (service *ClaimEvidenceService) CalculateState(ctx context.Context, command CalculateEvidenceStateCommand) (CalculateEvidenceStateResult, error) {
	if service == nil || service.repository == nil || command.MicroEventID <= 0 || command.ExpectedEventVersion <= 0 ||
		command.AlgorithmVersion != CanonicalEvidenceStateAlgorithmVersion || command.CalculatedAt.IsZero() {
		return CalculateEvidenceStateResult{}, ErrInvalidClaimEvidenceContract
	}
	target, err := service.repository.ReadEvidenceStateTarget(ctx, EvidenceStateTargetQuery{MicroEventID: command.MicroEventID,
		EventVersion: command.ExpectedEventVersion, AlgorithmVersion: command.AlgorithmVersion, CalculatedAt: command.CalculatedAt.UTC()})
	if err != nil {
		return CalculateEvidenceStateResult{}, fmt.Errorf("read evidence state target: %w", err)
	}
	if target.MicroEventID != command.MicroEventID || target.EventVersion != command.ExpectedEventVersion || target.ProfileID <= 0 ||
		target.AlgorithmVersion != command.AlgorithmVersion {
		return CalculateEvidenceStateResult{}, ErrInvalidClaimEvidenceContract
	}
	domainItems := make([]eventdomain.EvidenceStateItem, len(target.Items))
	ids := make([]int64, len(target.Items))
	for index, item := range target.Items {
		domainItems[index] = eventdomain.EvidenceStateItem{ClaimEvidenceVersionID: item.ClaimEvidenceVersionID,
			LineageRootID: item.LineageRootID, Relation: eventdomain.ClaimEvidenceRelation(item.Relation), Citable: item.Citable}
		ids[index] = item.ClaimEvidenceVersionID
	}
	calculated, err := eventdomain.CalculateEvidenceState(eventdomain.EvidenceStateInput{AlgorithmVersion: command.AlgorithmVersion, Items: domainItems})
	if err != nil {
		return CalculateEvidenceStateResult{}, fmt.Errorf("%w: %v", ErrInvalidClaimEvidenceContract, err)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	evidenceHash := evidenceStateSetHash(target.Items)
	snapshot, err := service.repository.CommitEvidenceStateSnapshot(ctx, CommitEvidenceStateSnapshotCommand{
		MicroEventID: target.MicroEventID, EventVersion: target.EventVersion, ProfileID: target.ProfileID,
		AlgorithmVersion: target.AlgorithmVersion, EvidenceSetHash: evidenceHash, State: string(calculated.State),
		IndependentOriginCount: calculated.IndependentOriginCount, ReasonCodes: calculated.ReasonCodes,
		ClaimEvidenceVersionIDs: ids, CalculatedAt: command.CalculatedAt.UTC(),
	})
	if err != nil {
		return CalculateEvidenceStateResult{}, fmt.Errorf("commit evidence state snapshot: %w", err)
	}
	if snapshot.ID <= 0 || snapshot.MicroEventID != target.MicroEventID || snapshot.EventVersion != target.EventVersion ||
		snapshot.ProfileID != target.ProfileID || snapshot.EvidenceSetHash != evidenceHash || snapshot.State != string(calculated.State) ||
		snapshot.IndependentOriginCount != calculated.IndependentOriginCount {
		return CalculateEvidenceStateResult{}, ErrInvalidClaimEvidenceContract
	}
	return CalculateEvidenceStateResult{Snapshot: snapshot}, nil
}

func canonicalClaimEvidenceCommand(command RecordClaimEvidenceCommand) (RecordClaimEvidenceCommand, error) {
	command.Subject, command.Predicate, command.Object = canonicalClaimPart(command.Subject), canonicalClaimPart(command.Predicate), canonicalClaimPart(command.Object)
	command.Relation, command.Origin = strings.TrimSpace(command.Relation), strings.TrimSpace(command.Origin)
	command.ExtractionSchemaVersion, command.IdempotencyKey = strings.TrimSpace(command.ExtractionSchemaVersion), strings.TrimSpace(command.IdempotencyKey)
	command.DecisionAt = command.DecisionAt.UTC()
	if command.MicroEventID <= 0 || command.ExpectedEventVersion <= 0 || command.DocumentVersionID <= 0 || command.TextQuoteSelectorID <= 0 ||
		command.Subject == "" || len(command.Subject) > 512 || command.Predicate == "" || len(command.Predicate) > 256 ||
		command.Object == "" || len(command.Object) > 2000 || !eventdomain.ClaimEvidenceRelation(command.Relation).Valid() ||
		command.ExtractionSchemaVersion != CanonicalClaimExtractionSchemaVersion || command.DecisionAt.IsZero() ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 96 || (command.Origin != "automatic" && command.Origin != "manual") ||
		(command.Origin == "automatic" && (command.ModelRunID == nil || *command.ModelRunID <= 0 || command.ActorUserID != nil)) ||
		(command.Origin == "manual" && (command.ActorUserID == nil || *command.ActorUserID <= 0)) ||
		(command.ModelRelationScore != nil && (*command.ModelRelationScore < 0 || *command.ModelRelationScore > 1)) {
		return RecordClaimEvidenceCommand{}, ErrInvalidClaimEvidenceContract
	}
	qualifiers := make([]ClaimQualifierDTO, len(command.Qualifiers))
	copy(qualifiers, command.Qualifiers)
	for index := range qualifiers {
		qualifiers[index].Key, qualifiers[index].Value = canonicalClaimPart(qualifiers[index].Key), canonicalClaimPart(qualifiers[index].Value)
		if qualifiers[index].Key == "" || len(qualifiers[index].Key) > 64 || qualifiers[index].Value == "" || len(qualifiers[index].Value) > 512 {
			return RecordClaimEvidenceCommand{}, ErrInvalidClaimEvidenceContract
		}
	}
	sort.Slice(qualifiers, func(left, right int) bool {
		return qualifiers[left].Key < qualifiers[right].Key || qualifiers[left].Key == qualifiers[right].Key && qualifiers[left].Value < qualifiers[right].Value
	})
	for index := 1; index < len(qualifiers); index++ {
		if qualifiers[index] == qualifiers[index-1] {
			return RecordClaimEvidenceCommand{}, ErrInvalidClaimEvidenceContract
		}
	}
	command.Qualifiers = qualifiers
	return command, nil
}

func canonicalClaimPart(value string) string {
	return norm.NFC.String(strings.Join(strings.Fields(value), " "))
}

func claimEvidenceClaimHash(subject, predicate, object string, qualifiers []ClaimQualifierDTO) string {
	payload, _ := json.Marshal(struct {
		Subject, Predicate, Object string
		Qualifiers                 []ClaimQualifierDTO
	}{subject, predicate, object, qualifiers})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func claimEvidenceCommandFingerprint(command RecordClaimEvidenceCommand, target ClaimEvidenceTargetDTO, claimHash string) string {
	payload, _ := json.Marshal(struct {
		Command   RecordClaimEvidenceCommand
		Target    ClaimEvidenceTargetDTO
		ClaimHash string
	}{command, target, claimHash})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func evidenceStateSetHash(items []EvidenceStateItemDTO) string {
	copyItems := append([]EvidenceStateItemDTO(nil), items...)
	sort.Slice(copyItems, func(left, right int) bool {
		return copyItems[left].ClaimEvidenceVersionID < copyItems[right].ClaimEvidenceVersionID
	})
	payload, _ := json.Marshal(copyItems)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func claimEvidenceReceiptMatches(result RecordClaimEvidenceResult, command RecordClaimEvidenceCommand, target ClaimEvidenceTargetDTO, claimHash string) bool {
	return result.Claim.ID > 0 && result.Claim.Version > 0 && result.Claim.MicroEventID == command.MicroEventID &&
		result.Claim.MicroEventVersion == command.ExpectedEventVersion && result.Claim.ClaimHash == claimHash &&
		result.Claim.Subject == command.Subject && result.Claim.Predicate == command.Predicate && result.Claim.Object == command.Object &&
		result.Evidence.ID > 0 && result.Evidence.ClaimID == result.Claim.ID && result.Evidence.DocumentVersionID == command.DocumentVersionID &&
		result.Evidence.TextQuoteSelectorID == command.TextQuoteSelectorID && result.Evidence.ContentFamilyID == target.ContentFamilyID &&
		result.Evidence.LineageRootID == target.LineageRootID && result.Evidence.Relation == command.Relation &&
		result.Evidence.ExtractionSchemaVersion == command.ExtractionSchemaVersion && result.Evidence.Origin == command.Origin
}

func lowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
