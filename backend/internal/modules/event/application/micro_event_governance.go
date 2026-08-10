package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
)

var ErrInvalidMicroEventGovernanceContract = errors.New("micro-event governance contract is invalid")
var ErrMicroEventGovernanceForbidden = errors.New("micro-event governance is forbidden")

const CanonicalMicroEventGovernanceProfileVersion = "micro-event-governance-v1"

type GovernMicroEventCommand struct {
	ActorUserID                int64
	Action                     string
	MicroEventID               int64
	ExpectedEventVersion       int64
	MembershipDecisionID       int64
	ContentFamilyID            int64
	ExpectedMemberVersion      int64
	TargetMicroEventID         int64
	ExpectedTargetEventVersion int64
	ReasonCode                 string
	Note                       string
	GovernanceProfileVersion   string
	IdempotencyKey             string
}

type ApplyMicroEventGovernanceCommand struct {
	GovernMicroEventCommand
	CommandFingerprint string
}

type MicroEventGovernanceFeedbackDTO struct {
	ID                         int64
	Action                     string
	ActorUserID                int64
	MicroEventID               int64
	OriginalEventVersion       int64
	MembershipDecisionID       int64
	ContentFamilyID            int64
	TargetMicroEventID         int64
	TargetEventVersion         int64
	ResultMicroEventID         int64
	ResultEventVersion         int64
	ResultTargetMicroEventID   int64
	ResultTargetEventVersion   int64
	ResultMembershipDecisionID int64
	ResultMemberVersion        int64
	GovernanceProfileVersion   string
	ReasonCode                 string
	Note                       string
	IdempotencyKey             string
}

type ApplyMicroEventGovernanceResult struct {
	Feedback    MicroEventGovernanceFeedbackDTO
	SourceEvent MicroEventDTO
	TargetEvent *MicroEventDTO
}

type GovernMicroEventResult = ApplyMicroEventGovernanceResult

type MicroEventGovernanceRepository interface {
	ApplyMicroEventGovernance(context.Context, ApplyMicroEventGovernanceCommand) (ApplyMicroEventGovernanceResult, error)
}

type MicroEventGovernanceService struct {
	repository MicroEventGovernanceRepository
}

func NewMicroEventGovernanceService(repository MicroEventGovernanceRepository) (*MicroEventGovernanceService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidMicroEventGovernanceContract)
	}
	return &MicroEventGovernanceService{repository: repository}, nil
}

func (service *MicroEventGovernanceService) Govern(ctx context.Context, command GovernMicroEventCommand) (GovernMicroEventResult, error) {
	if service == nil || service.repository == nil || command.ActorUserID <= 0 ||
		command.GovernanceProfileVersion != CanonicalMicroEventGovernanceProfileVersion ||
		strings.TrimSpace(command.ReasonCode) == "" || len(command.ReasonCode) > 64 || len(command.Note) > 1000 ||
		strings.TrimSpace(command.IdempotencyKey) == "" || len(command.IdempotencyKey) > 96 {
		return GovernMicroEventResult{}, ErrInvalidMicroEventGovernanceContract
	}
	if err := eventdomain.ValidateMicroEventGovernance(eventdomain.MicroEventGovernanceInput{Action: command.Action,
		MicroEventID: command.MicroEventID, ExpectedEventVersion: command.ExpectedEventVersion,
		MembershipDecisionID: command.MembershipDecisionID, ContentFamilyID: command.ContentFamilyID,
		ExpectedMemberVersion: command.ExpectedMemberVersion, TargetMicroEventID: command.TargetMicroEventID,
		ExpectedTargetEventVersion: command.ExpectedTargetEventVersion}); err != nil {
		return GovernMicroEventResult{}, fmt.Errorf("%w: %v", ErrInvalidMicroEventGovernanceContract, err)
	}
	mutation := ApplyMicroEventGovernanceCommand{GovernMicroEventCommand: command,
		CommandFingerprint: microEventGovernanceFingerprint(command)}
	result, err := service.repository.ApplyMicroEventGovernance(ctx, mutation)
	if err != nil {
		return GovernMicroEventResult{}, fmt.Errorf("apply micro-event governance: %w", err)
	}
	if !microEventGovernanceReceiptMatches(result, mutation) {
		return GovernMicroEventResult{}, fmt.Errorf("%w: governance receipt changed", ErrInvalidMicroEventGovernanceContract)
	}
	return result, nil
}

func microEventGovernanceFingerprint(command GovernMicroEventCommand) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%d|%d|%d|%d|%d|%d|%s|%s|%s", command.ActorUserID,
		command.Action, command.MicroEventID, command.ExpectedEventVersion, command.MembershipDecisionID,
		command.ContentFamilyID, command.ExpectedMemberVersion, command.TargetMicroEventID,
		command.ExpectedTargetEventVersion, command.ReasonCode, command.Note, command.GovernanceProfileVersion)))
	return hex.EncodeToString(digest[:])
}

func microEventGovernanceReceiptMatches(value ApplyMicroEventGovernanceResult, command ApplyMicroEventGovernanceCommand) bool {
	feedback := value.Feedback
	if feedback.ID <= 0 || feedback.Action != command.Action || feedback.ActorUserID != command.ActorUserID ||
		feedback.MicroEventID != command.MicroEventID || feedback.OriginalEventVersion != command.ExpectedEventVersion ||
		feedback.MembershipDecisionID != command.MembershipDecisionID || feedback.ContentFamilyID != command.ContentFamilyID ||
		feedback.TargetMicroEventID != command.TargetMicroEventID || feedback.TargetEventVersion != command.ExpectedTargetEventVersion ||
		feedback.GovernanceProfileVersion != command.GovernanceProfileVersion || feedback.ReasonCode != command.ReasonCode ||
		feedback.Note != command.Note || feedback.IdempotencyKey != command.IdempotencyKey ||
		value.SourceEvent.ID != feedback.ResultMicroEventID || value.SourceEvent.Version != feedback.ResultEventVersion {
		return false
	}
	if feedback.ResultTargetEventVersion > 0 {
		return value.TargetEvent != nil && value.TargetEvent.ID == feedback.ResultTargetMicroEventID &&
			value.TargetEvent.Version == feedback.ResultTargetEventVersion
	}
	return value.TargetEvent == nil
}
