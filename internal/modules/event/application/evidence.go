package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type EvidenceInput struct {
	Event                domain.Event
	ValidMembers         []domain.EventMember
	IndependentSourceIDs []int64
	AuthoritativeSource  bool
	ManualConfirmed      bool
}

type EvidenceRecomputeCommand struct {
	EventID     int64
	ReasonCode  string
	ActorUserID *int64
}

func (command EvidenceRecomputeCommand) Validate() error {
	if command.EventID <= 0 || !domain.ValidReasonCode(command.ReasonCode) {
		return fmt.Errorf("invalid evidence recompute command")
	}
	return nil
}

type EvidenceRepository interface {
	RecomputeEventEvidence(context.Context, EvidenceRecomputeCommand) (domain.Event, error)
}

type EvidenceService struct {
	repository EvidenceRepository
}

func NewEvidenceService(repository EvidenceRepository) *EvidenceService {
	return &EvidenceService{repository: repository}
}

func (service *EvidenceService) Recompute(ctx context.Context, command EvidenceRecomputeCommand) (domain.Event, error) {
	if service == nil || service.repository == nil {
		return domain.Event{}, fmt.Errorf("event evidence repository is required")
	}
	if err := command.Validate(); err != nil {
		return domain.Event{}, err
	}
	return service.repository.RecomputeEventEvidence(ctx, command)
}

func RecomputeEvidence(input EvidenceInput) (domain.LifecycleStatus, *int64, error) {
	if err := input.Event.Validate(); err != nil {
		return "", nil, err
	}
	for _, member := range input.ValidMembers {
		if err := member.Validate(); err != nil {
			return "", nil, err
		}
	}
	if input.Event.ManualLocked {
		return input.Event.LifecycleStatus, input.Event.RepresentativeContentID, nil
	}
	if len(input.ValidMembers) == 0 {
		return domain.LifecycleRejected, nil, nil
	}
	representative := selectRepresentative(input.ValidMembers)
	if input.ManualConfirmed || input.AuthoritativeSource || len(uniquePositive(input.IndependentSourceIDs)) >= 2 {
		return domain.LifecycleActive, &representative.ContentID, nil
	}
	if input.Event.LifecycleStatus == domain.LifecycleActive || input.Event.LifecycleStatus == domain.LifecycleCooling || input.Event.LifecycleStatus == domain.LifecycleClosed {
		return domain.LifecycleCooling, &representative.ContentID, nil
	}
	return domain.LifecycleDetected, &representative.ContentID, nil
}

func selectRepresentative(members []domain.EventMember) domain.EventMember {
	var selected *domain.EventMember
	for index := range members {
		member := &members[index]
		if !member.ManualLocked {
			continue
		}
		if selected == nil || member.ContentID < selected.ContentID {
			selected = member
		}
	}
	if selected != nil {
		return *selected
	}
	result := members[0]
	for _, member := range members[1:] {
		if member.MembershipScore > result.MembershipScore || member.MembershipScore == result.MembershipScore && member.ContentID < result.ContentID {
			result = member
		}
	}
	return result
}

func uniquePositive(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	return result
}

func validateEvidenceStatus(status domain.LifecycleStatus) error {
	if !status.Valid() {
		return fmt.Errorf("invalid evidence status")
	}
	return nil
}
