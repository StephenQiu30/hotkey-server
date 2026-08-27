package application

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidAcceptedMatchProjectionContract = errors.New("accepted match event projection contract is invalid")

type ResolveAcceptedMatchFamilyQuery struct {
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
}

type AcceptedMatchFamilyDTO struct {
	ContentFamilyID         int64
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
	EffectiveDecision       string
}

type AcceptedMatchFamilyReader interface {
	ResolveAcceptedMatchFamily(context.Context, ResolveAcceptedMatchFamilyQuery) (AcceptedMatchFamilyDTO, error)
}

type ProjectAcceptedDocumentMatchCommand struct {
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
}

type ProjectAcceptedDocumentMatchResult struct {
	MicroEvent      MicroEventDTO
	Membership      MicroEventMembershipDecisionDTO
	Storyline       *StorylineDTO
	StorylineEvent  *StorylineEventDTO
	HeatSnapshots   []EventHeatSnapshotDTO
	HeatUnavailable bool
}

type AcceptedMatchEventProjectionService struct {
	families    AcceptedMatchFamilyReader
	microEvents *MicroEventService
}

func NewAcceptedMatchEventProjectionService(families AcceptedMatchFamilyReader,
	microEvents *MicroEventService) (*AcceptedMatchEventProjectionService, error) {
	if families == nil || microEvents == nil {
		return nil, fmt.Errorf("%w: dependencies are required", ErrInvalidAcceptedMatchProjectionContract)
	}
	return &AcceptedMatchEventProjectionService{families: families, microEvents: microEvents}, nil
}

func (service *AcceptedMatchEventProjectionService) Project(ctx context.Context, command ProjectAcceptedDocumentMatchCommand) (ProjectAcceptedDocumentMatchResult, error) {
	if service == nil || service.families == nil || service.microEvents == nil ||
		command.DocumentMatchDecisionID <= 0 || command.DocumentVersionID <= 0 {
		return ProjectAcceptedDocumentMatchResult{}, ErrInvalidAcceptedMatchProjectionContract
	}
	family, err := service.families.ResolveAcceptedMatchFamily(ctx, ResolveAcceptedMatchFamilyQuery{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID, DocumentVersionID: command.DocumentVersionID})
	if err != nil {
		return ProjectAcceptedDocumentMatchResult{}, fmt.Errorf("resolve accepted match family: %w", err)
	}
	if family.ContentFamilyID <= 0 || family.DocumentMatchDecisionID != command.DocumentMatchDecisionID ||
		family.DocumentVersionID != command.DocumentVersionID || family.EffectiveDecision != "accepted" {
		return ProjectAcceptedDocumentMatchResult{}, ErrInvalidAcceptedMatchProjectionContract
	}
	assigned, err := service.microEvents.Assign(ctx, AssignContentFamilyToMicroEventCommand{ContentFamilyID: family.ContentFamilyID,
		DocumentMatchDecisionID:  family.DocumentMatchDecisionID,
		ClusteringProfileVersion: CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		return ProjectAcceptedDocumentMatchResult{}, fmt.Errorf("assign accepted match to micro-event: %w", err)
	}
	result := ProjectAcceptedDocumentMatchResult{MicroEvent: assigned.Event, Membership: assigned.Decision,
		HeatSnapshots: []EventHeatSnapshotDTO{}}
	return result, nil
}
