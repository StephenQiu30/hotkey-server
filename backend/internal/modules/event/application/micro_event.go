package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
)

var ErrInvalidMicroEventContract = errors.New("micro-event contract is invalid")

const CanonicalMicroEventClusteringProfileVersion = "same-event-cold-start-v1"

type MicroEventFeaturesDTO struct {
	SparseSimilarity      float64
	DenseSimilarity       float64
	EntityOverlap         float64
	ActionOverlap         float64
	LocationConsistency   float64
	IdentifierConsistency float64
	TimeSimilarity        float64
	LineageRelation       float64
}

type MicroEventCandidateDTO struct {
	MicroEventID        int64
	EventVersion        int64
	Features            MicroEventFeaturesDTO
	DenseAvailable      bool
	HardConflict        bool
	HardConflictReasons []string
}

type ReadMicroEventAssignmentTargetQuery struct {
	ContentFamilyID          int64
	DocumentMatchDecisionID  int64
	ClusteringProfileVersion string
}

type MicroEventAssignmentTargetDTO struct {
	ContentFamilyID         int64
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
	MonitorID               int64
	MonitorVersionID        int64
	EffectiveMatchDecision  string
	PrimarySubjectKey       string
	PrimaryActionKey        string
	LocationKeys            []string
	IdentifierKeys          []string
	OccurredAt              time.Time
	Candidates              []MicroEventCandidateDTO
	ExistingAssignment      *CommitMicroEventMembershipResult
}

type CommitMicroEventMembershipCommand struct {
	ContentFamilyID          int64
	DocumentMatchDecisionID  int64
	MonitorID                int64
	MonitorVersionID         int64
	EventKey                 string
	PrimarySubjectKey        string
	PrimaryActionKey         string
	LocationKeys             []string
	IdentifierKeys           []string
	OccurredAt               time.Time
	Action                   string
	CandidateMicroEventID    int64
	ExpectedEventVersion     int64
	SameEventScore           float64
	LeadingMargin            float64
	Features                 MicroEventFeaturesDTO
	HardConflictReasons      []string
	ClusteringProfileVersion string
	ReasonCodes              []string
	IdempotencyKey           string
	CommandFingerprint       string
}

type MicroEventDTO struct {
	ID                       int64
	Version                  int64
	EventKey                 string
	Status                   string
	PrimarySubjectKey        string
	PrimaryActionKey         string
	LocationKeys             []string
	IdentifierKeys           []string
	EventStartedAt           time.Time
	ClusteringProfileVersion string
}

type MicroEventMembershipDecisionDTO struct {
	ID                       int64
	ContentFamilyID          int64
	DocumentMatchDecisionID  int64
	MicroEventID             int64
	EventVersion             int64
	Action                   string
	SameEventScore           float64
	LeadingMargin            float64
	Features                 MicroEventFeaturesDTO
	ClusteringProfileVersion string
	ReasonCodes              []string
}

type CommitMicroEventMembershipResult struct {
	Event    MicroEventDTO
	Decision MicroEventMembershipDecisionDTO
}

type AssignContentFamilyToMicroEventCommand struct {
	ContentFamilyID          int64
	DocumentMatchDecisionID  int64
	ClusteringProfileVersion string
}

type AssignContentFamilyToMicroEventResult struct {
	Event    MicroEventDTO
	Decision MicroEventMembershipDecisionDTO
}

type MicroEventRepository interface {
	ReadMicroEventAssignmentTarget(context.Context, ReadMicroEventAssignmentTargetQuery) (MicroEventAssignmentTargetDTO, error)
	CommitMicroEventMembership(context.Context, CommitMicroEventMembershipCommand) (CommitMicroEventMembershipResult, error)
}

type MicroEventQualityProfileReader interface {
	IsDecisionQualityProfileActive(context.Context, string, string) (bool, error)
}

type MicroEventService struct {
	repository      MicroEventRepository
	qualityProfiles MicroEventQualityProfileReader
}

func NewMicroEventService(repository MicroEventRepository) (*MicroEventService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidMicroEventContract)
	}
	return &MicroEventService{repository: repository}, nil
}

func NewMicroEventServiceWithQualityProfiles(repository MicroEventRepository, profiles MicroEventQualityProfileReader) (*MicroEventService, error) {
	if profiles == nil {
		return nil, fmt.Errorf("%w: quality profile reader is required", ErrInvalidMicroEventContract)
	}
	service, err := NewMicroEventService(repository)
	if err != nil {
		return nil, err
	}
	service.qualityProfiles = profiles
	return service, nil
}

func (service *MicroEventService) Assign(ctx context.Context, command AssignContentFamilyToMicroEventCommand) (AssignContentFamilyToMicroEventResult, error) {
	if service == nil || service.repository == nil || command.ContentFamilyID <= 0 || command.DocumentMatchDecisionID <= 0 ||
		command.ClusteringProfileVersion != CanonicalMicroEventClusteringProfileVersion {
		return AssignContentFamilyToMicroEventResult{}, ErrInvalidMicroEventContract
	}
	target, err := service.repository.ReadMicroEventAssignmentTarget(ctx, ReadMicroEventAssignmentTargetQuery{
		ContentFamilyID: command.ContentFamilyID, DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		ClusteringProfileVersion: command.ClusteringProfileVersion,
	})
	if err != nil {
		return AssignContentFamilyToMicroEventResult{}, fmt.Errorf("read micro-event assignment target: %w", err)
	}
	if err := validateMicroEventTarget(target, command); err != nil {
		return AssignContentFamilyToMicroEventResult{}, fmt.Errorf("%w: %v", ErrInvalidMicroEventContract, err)
	}
	if target.ExistingAssignment != nil {
		value := target.ExistingAssignment
		if !microEventExistingAssignmentMatches(*value, target, command.ClusteringProfileVersion) {
			return AssignContentFamilyToMicroEventResult{}, ErrInvalidMicroEventContract
		}
		return AssignContentFamilyToMicroEventResult{Event: value.Event, Decision: value.Decision}, nil
	}
	candidates := make([]eventdomain.MicroEventCandidate, len(target.Candidates))
	for index, candidate := range target.Candidates {
		features := microEventFeaturesFromDTO(candidate.Features)
		if err := features.Validate(); err != nil {
			return AssignContentFamilyToMicroEventResult{}, ErrInvalidMicroEventContract
		}
		candidates[index] = eventdomain.MicroEventCandidate{MicroEventID: candidate.MicroEventID, EventVersion: candidate.EventVersion,
			Features: features, DenseAvailable: candidate.DenseAvailable, HardConflict: candidate.HardConflict,
			HardConflictReasons: append([]string(nil), candidate.HardConflictReasons...)}
	}
	decision, err := eventdomain.DecideMicroEventMembership(eventdomain.MicroEventDecisionInput{
		ContentFamilyID: target.ContentFamilyID, Candidates: candidates, ProfileVersion: command.ClusteringProfileVersion,
	})
	if err != nil {
		return AssignContentFamilyToMicroEventResult{}, fmt.Errorf("%w: %v", ErrInvalidMicroEventContract, err)
	}
	if decision.Action == eventdomain.MicroEventActionJoin && service.qualityProfiles != nil {
		active, readErr := service.qualityProfiles.IsDecisionQualityProfileActive(ctx, "micro_event_clustering", command.ClusteringProfileVersion)
		if readErr != nil || !active {
			decision.Action = eventdomain.MicroEventActionReview
			decision.ReasonCodes = append(decision.ReasonCodes, "quality_profile_not_active")
		}
	}
	mutation := CommitMicroEventMembershipCommand{
		ContentFamilyID: target.ContentFamilyID, DocumentMatchDecisionID: target.DocumentMatchDecisionID,
		MonitorID: target.MonitorID, MonitorVersionID: target.MonitorVersionID,
		EventKey:          microEventKey(target.ContentFamilyID, command.ClusteringProfileVersion),
		PrimarySubjectKey: target.PrimarySubjectKey, PrimaryActionKey: target.PrimaryActionKey,
		LocationKeys: append([]string(nil), target.LocationKeys...), IdentifierKeys: append([]string(nil), target.IdentifierKeys...),
		OccurredAt: target.OccurredAt.UTC(), Action: string(decision.Action), CandidateMicroEventID: decision.MicroEventID,
		ExpectedEventVersion: decision.EventVersion, SameEventScore: roundMicroEventScore(decision.SameEventScore),
		LeadingMargin: roundMicroEventScore(decision.LeadingMargin),
		Features:      roundMicroEventFeatures(microEventFeaturesDTO(decision.Features)), ClusteringProfileVersion: decision.ProfileVersion,
		ReasonCodes: append([]string(nil), decision.ReasonCodes...),
	}
	if decision.MicroEventID > 0 {
		for _, candidate := range target.Candidates {
			if candidate.MicroEventID == decision.MicroEventID {
				mutation.HardConflictReasons = append([]string(nil), candidate.HardConflictReasons...)
				break
			}
		}
	}
	mutation.IdempotencyKey, mutation.CommandFingerprint = microEventMutationIdentity(mutation)
	persisted, err := service.repository.CommitMicroEventMembership(ctx, mutation)
	if err != nil {
		return AssignContentFamilyToMicroEventResult{}, fmt.Errorf("commit micro-event membership: %w", err)
	}
	if !microEventReceiptMatches(persisted, target, mutation) {
		return AssignContentFamilyToMicroEventResult{}, fmt.Errorf("%w: micro-event receipt changed", ErrInvalidMicroEventContract)
	}
	return AssignContentFamilyToMicroEventResult{Event: persisted.Event, Decision: persisted.Decision}, nil
}

func microEventExistingAssignmentMatches(value CommitMicroEventMembershipResult, target MicroEventAssignmentTargetDTO, profileVersion string) bool {
	return value.Event.ID > 0 && value.Event.Version > 0 && value.Event.ClusteringProfileVersion == profileVersion &&
		value.Decision.ID > 0 && value.Decision.ContentFamilyID == target.ContentFamilyID &&
		value.Decision.DocumentMatchDecisionID > 0 && value.Decision.MicroEventID == value.Event.ID &&
		value.Decision.EventVersion == value.Event.Version && value.Decision.ClusteringProfileVersion == profileVersion &&
		(value.Decision.Action == string(eventdomain.MicroEventActionCreate) ||
			value.Decision.Action == string(eventdomain.MicroEventActionJoin) ||
			value.Decision.Action == string(eventdomain.MicroEventActionReview))
}

func validateMicroEventTarget(target MicroEventAssignmentTargetDTO, command AssignContentFamilyToMicroEventCommand) error {
	if target.ContentFamilyID != command.ContentFamilyID || target.DocumentMatchDecisionID != command.DocumentMatchDecisionID ||
		target.DocumentVersionID <= 0 || target.MonitorID <= 0 || target.MonitorVersionID <= 0 || target.EffectiveMatchDecision != "accepted" ||
		strings.TrimSpace(target.PrimarySubjectKey) == "" || strings.TrimSpace(target.PrimaryActionKey) == "" || target.OccurredAt.IsZero() ||
		len(target.Candidates) > 20 {
		return errors.New("assignment target is not an accepted structured match")
	}
	return nil
}

func microEventFeaturesFromDTO(value MicroEventFeaturesDTO) eventdomain.MicroEventFeatures {
	return eventdomain.MicroEventFeatures{SparseSimilarity: value.SparseSimilarity, DenseSimilarity: value.DenseSimilarity,
		EntityOverlap: value.EntityOverlap, ActionOverlap: value.ActionOverlap, LocationConsistency: value.LocationConsistency,
		IdentifierConsistency: value.IdentifierConsistency, TimeSimilarity: value.TimeSimilarity, LineageRelation: value.LineageRelation}
}

func microEventFeaturesDTO(value eventdomain.MicroEventFeatures) MicroEventFeaturesDTO {
	return MicroEventFeaturesDTO{SparseSimilarity: value.SparseSimilarity, DenseSimilarity: value.DenseSimilarity,
		EntityOverlap: value.EntityOverlap, ActionOverlap: value.ActionOverlap, LocationConsistency: value.LocationConsistency,
		IdentifierConsistency: value.IdentifierConsistency, TimeSimilarity: value.TimeSimilarity, LineageRelation: value.LineageRelation}
}

func microEventKey(contentFamilyID int64, profile string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("micro-event:%d:%s", contentFamilyID, profile)))
	return hex.EncodeToString(digest[:])
}

func microEventMutationIdentity(command CommitMicroEventMembershipCommand) (string, string) {
	idempotencyDigest := sha256.Sum256([]byte(fmt.Sprintf("micro-event:%d:%s", command.ContentFamilyID, command.ClusteringProfileVersion)))
	fingerprintDigest := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%d|%d|%s|%s|%s|%s|%s",
		command.ContentFamilyID, command.DocumentMatchDecisionID, command.MonitorID, command.MonitorVersionID,
		command.PrimarySubjectKey, command.PrimaryActionKey, command.OccurredAt.UTC().Format(time.RFC3339Nano),
		command.ClusteringProfileVersion, command.EventKey)))
	return "micro-event-" + hex.EncodeToString(idempotencyDigest[:16]), hex.EncodeToString(fingerprintDigest[:])
}

func microEventReceiptMatches(value CommitMicroEventMembershipResult, target MicroEventAssignmentTargetDTO, command CommitMicroEventMembershipCommand) bool {
	return value.Event.ID > 0 && value.Event.Version > 0 && value.Event.EventKey != "" && value.Event.PrimarySubjectKey == target.PrimarySubjectKey &&
		value.Event.PrimaryActionKey == target.PrimaryActionKey && value.Event.ClusteringProfileVersion == command.ClusteringProfileVersion &&
		value.Decision.ID > 0 && value.Decision.ContentFamilyID == target.ContentFamilyID &&
		value.Decision.DocumentMatchDecisionID == target.DocumentMatchDecisionID && value.Decision.MicroEventID == value.Event.ID &&
		value.Decision.EventVersion == value.Event.Version && value.Decision.Action == command.Action &&
		value.Decision.SameEventScore == command.SameEventScore && value.Decision.LeadingMargin == command.LeadingMargin &&
		value.Decision.Features == command.Features && value.Decision.ClusteringProfileVersion == command.ClusteringProfileVersion
}

func roundMicroEventScore(value float64) float64 { return math.Round(value*1e7) / 1e7 }

func roundMicroEventFeatures(value MicroEventFeaturesDTO) MicroEventFeaturesDTO {
	value.SparseSimilarity = roundMicroEventScore(value.SparseSimilarity)
	value.DenseSimilarity = roundMicroEventScore(value.DenseSimilarity)
	value.EntityOverlap = roundMicroEventScore(value.EntityOverlap)
	value.ActionOverlap = roundMicroEventScore(value.ActionOverlap)
	value.LocationConsistency = roundMicroEventScore(value.LocationConsistency)
	value.IdentifierConsistency = roundMicroEventScore(value.IdentifierConsistency)
	value.TimeSimilarity = roundMicroEventScore(value.TimeSimilarity)
	value.LineageRelation = roundMicroEventScore(value.LineageRelation)
	return value
}
