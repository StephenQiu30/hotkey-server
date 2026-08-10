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

var ErrInvalidStorylineContract = errors.New("storyline contract is invalid")

const CanonicalStorylineRelationProfileVersion = "storyline-relation-v1"

type StorylineCandidateDTO struct {
	StorylineID       int64
	StorylineVersion  int64
	SubjectSimilarity float64
	ActionOverlap     float64
	TimeRecency       float64
	LatestEventAt     time.Time
}

type ReadStorylineAssignmentTargetQuery struct {
	MicroEventID           int64
	MicroEventVersion      int64
	RelationProfileVersion string
}

type StorylineAssignmentTargetDTO struct {
	MicroEventID           int64
	MicroEventVersion      int64
	MicroEventStatus       string
	PrimarySubjectKey      string
	PrimaryActionKey       string
	EventStartedAt         time.Time
	RelationProfileVersion string
	Candidates             []StorylineCandidateDTO
	ExistingAssignment     *CommitStorylineAssignmentResult
}

type CommitStorylineAssignmentCommand struct {
	MicroEventID             int64
	MicroEventVersion        int64
	StorylineKey             string
	Title                    string
	CreateNew                bool
	CandidateStorylineID     int64
	ExpectedStorylineVersion int64
	RelationType             string
	RelationScore            float64
	RelationProfileVersion   string
	ReasonCodes              []string
	IdempotencyKey           string
	CommandFingerprint       string
}

type StorylineDTO struct {
	ID                     int64
	Version                int64
	StorylineKey           string
	Title                  string
	Summary                string
	Status                 string
	RelationProfileVersion string
}

type StorylineEventDTO struct {
	ID                     int64
	StorylineID            int64
	StorylineVersion       int64
	MicroEventID           int64
	MicroEventVersion      int64
	RelationType           string
	RelationScore          float64
	RelationProfileVersion string
	ReasonCodes            []string
	DecisionOrigin         string
}

type CommitStorylineAssignmentResult struct {
	Storyline StorylineDTO
	Relation  StorylineEventDTO
}

type AssignMicroEventToStorylineCommand struct {
	MicroEventID           int64
	MicroEventVersion      int64
	RelationProfileVersion string
}

type AssignMicroEventToStorylineResult struct {
	Storyline StorylineDTO
	Relation  StorylineEventDTO
}

type StorylineRepository interface {
	ReadStorylineAssignmentTarget(context.Context, ReadStorylineAssignmentTargetQuery) (StorylineAssignmentTargetDTO, error)
	CommitStorylineAssignment(context.Context, CommitStorylineAssignmentCommand) (CommitStorylineAssignmentResult, error)
}

type StorylineService struct{ repository StorylineRepository }

func NewStorylineService(repository StorylineRepository) (*StorylineService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidStorylineContract)
	}
	return &StorylineService{repository: repository}, nil
}

func (service *StorylineService) Assign(ctx context.Context, command AssignMicroEventToStorylineCommand) (AssignMicroEventToStorylineResult, error) {
	if service == nil || service.repository == nil || command.MicroEventID <= 0 || command.MicroEventVersion <= 0 ||
		command.RelationProfileVersion != CanonicalStorylineRelationProfileVersion {
		return AssignMicroEventToStorylineResult{}, ErrInvalidStorylineContract
	}
	target, err := service.repository.ReadStorylineAssignmentTarget(ctx, ReadStorylineAssignmentTargetQuery{
		MicroEventID: command.MicroEventID, MicroEventVersion: command.MicroEventVersion,
		RelationProfileVersion: command.RelationProfileVersion,
	})
	if err != nil {
		return AssignMicroEventToStorylineResult{}, fmt.Errorf("read storyline assignment target: %w", err)
	}
	if !validStorylineTarget(target, command) {
		return AssignMicroEventToStorylineResult{}, ErrInvalidStorylineContract
	}
	if target.ExistingAssignment != nil {
		if target.ExistingAssignment.Relation.MicroEventID != command.MicroEventID ||
			target.ExistingAssignment.Relation.MicroEventVersion != command.MicroEventVersion ||
			target.ExistingAssignment.Relation.RelationProfileVersion != command.RelationProfileVersion {
			return AssignMicroEventToStorylineResult{}, ErrInvalidStorylineContract
		}
		return AssignMicroEventToStorylineResult{Storyline: target.ExistingAssignment.Storyline,
			Relation: target.ExistingAssignment.Relation}, nil
	}
	candidates := make([]eventdomain.StorylineCandidate, len(target.Candidates))
	for index, candidate := range target.Candidates {
		candidates[index] = eventdomain.StorylineCandidate{StorylineID: candidate.StorylineID,
			StorylineVersion: candidate.StorylineVersion, SubjectSimilarity: candidate.SubjectSimilarity,
			ActionOverlap: candidate.ActionOverlap, TimeRecency: candidate.TimeRecency,
			LatestEventAt: candidate.LatestEventAt.UTC()}
	}
	decision, err := eventdomain.DecideStorylineRelation(eventdomain.StorylineRelationInput{
		MicroEventID: target.MicroEventID, MicroEventTime: target.EventStartedAt.UTC(),
		ProfileVersion: target.RelationProfileVersion, Candidates: candidates,
	})
	if err != nil {
		return AssignMicroEventToStorylineResult{}, fmt.Errorf("%w: %v", ErrInvalidStorylineContract, err)
	}
	mutation := CommitStorylineAssignmentCommand{MicroEventID: target.MicroEventID,
		MicroEventVersion: target.MicroEventVersion, StorylineKey: storylineKey(target.MicroEventID, target.RelationProfileVersion),
		Title: target.PrimarySubjectKey, CreateNew: decision.CreateNew,
		CandidateStorylineID: decision.StorylineID, ExpectedStorylineVersion: decision.StorylineVersion,
		RelationType: decision.RelationType, RelationScore: roundStorylineScore(decision.RelationScore),
		RelationProfileVersion: target.RelationProfileVersion, ReasonCodes: append([]string(nil), decision.ReasonCodes...)}
	mutation.IdempotencyKey, mutation.CommandFingerprint = storylineMutationIdentity(mutation)
	persisted, err := service.repository.CommitStorylineAssignment(ctx, mutation)
	if err != nil {
		return AssignMicroEventToStorylineResult{}, fmt.Errorf("commit storyline assignment: %w", err)
	}
	if !storylineReceiptMatches(persisted, mutation) {
		return AssignMicroEventToStorylineResult{}, fmt.Errorf("%w: storyline receipt changed", ErrInvalidStorylineContract)
	}
	return AssignMicroEventToStorylineResult{Storyline: persisted.Storyline, Relation: persisted.Relation}, nil
}

func validStorylineTarget(value StorylineAssignmentTargetDTO, command AssignMicroEventToStorylineCommand) bool {
	return value.MicroEventID == command.MicroEventID && value.MicroEventVersion == command.MicroEventVersion &&
		(value.MicroEventStatus == "active" || value.MicroEventStatus == "review_pending") &&
		strings.TrimSpace(value.PrimarySubjectKey) != "" && strings.TrimSpace(value.PrimaryActionKey) != "" &&
		!value.EventStartedAt.IsZero() && value.RelationProfileVersion == command.RelationProfileVersion && len(value.Candidates) <= 20
}

func storylineKey(microEventID int64, profile string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("storyline:%d:%s", microEventID, profile)))
	return hex.EncodeToString(digest[:])
}

func storylineMutationIdentity(command CommitStorylineAssignmentCommand) (string, string) {
	idempotencyDigest := sha256.Sum256([]byte(fmt.Sprintf("storyline-event:%d:%d:%s", command.MicroEventID,
		command.MicroEventVersion, command.RelationProfileVersion)))
	fingerprintDigest := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%t|%d|%d|%s|%.7f|%s|%s", command.MicroEventID,
		command.MicroEventVersion, command.CreateNew, command.CandidateStorylineID, command.ExpectedStorylineVersion,
		command.RelationType, command.RelationScore, command.Title, command.RelationProfileVersion)))
	return "storyline-event-" + hex.EncodeToString(idempotencyDigest[:16]), hex.EncodeToString(fingerprintDigest[:])
}

func storylineReceiptMatches(value CommitStorylineAssignmentResult, command CommitStorylineAssignmentCommand) bool {
	return value.Storyline.ID > 0 && value.Storyline.Version == value.Relation.StorylineVersion &&
		value.Storyline.RelationProfileVersion == command.RelationProfileVersion && value.Relation.ID > 0 &&
		value.Relation.StorylineID == value.Storyline.ID && value.Relation.MicroEventID == command.MicroEventID &&
		value.Relation.MicroEventVersion == command.MicroEventVersion && value.Relation.RelationType == command.RelationType &&
		value.Relation.RelationScore == command.RelationScore && value.Relation.RelationProfileVersion == command.RelationProfileVersion &&
		value.Relation.DecisionOrigin == "automatic"
}

func roundStorylineScore(value float64) float64 { return math.Round(value*1e7) / 1e7 }
