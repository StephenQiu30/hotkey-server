package application

import (
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
	representative := input.ValidMembers[0]
	for _, member := range input.ValidMembers[1:] {
		if member.ManualLocked {
			continue
		}
		if member.MembershipScore > representative.MembershipScore || member.MembershipScore == representative.MembershipScore && member.ContentID < representative.ContentID {
			representative = member
		}
	}
	if input.ManualConfirmed || input.AuthoritativeSource || len(uniquePositive(input.IndependentSourceIDs)) >= 2 {
		return domain.LifecycleActive, &representative.ContentID, nil
	}
	if input.Event.LifecycleStatus == domain.LifecycleClosed {
		return domain.LifecycleCooling, &representative.ContentID, nil
	}
	return domain.LifecycleDetected, &representative.ContentID, nil
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
