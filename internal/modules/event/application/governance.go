package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type MergeCommand struct {
	SourceEventID, TargetEventID int64
	SourceExpectedVersion        int64
	TargetExpectedVersion        int64
	ActorUserID                  *int64
	ReasonCode                   string
}

func (command MergeCommand) Validate() error {
	if command.SourceEventID <= 0 || command.TargetEventID <= 0 || command.SourceEventID == command.TargetEventID || command.SourceExpectedVersion <= 0 || command.TargetExpectedVersion <= 0 || !domain.ValidReasonCode(command.ReasonCode) {
		return fmt.Errorf("invalid merge command")
	}
	return nil
}

type SplitMember struct {
	ContentID       int64
	ExpectedVersion int64
}

type SplitCommand struct {
	SourceEventID         int64
	SourceExpectedVersion int64
	Members               []SplitMember
	ActorUserID           *int64
	ReasonCode            string
}

func (command SplitCommand) Validate() error {
	if command.SourceEventID <= 0 || command.SourceExpectedVersion <= 0 || len(command.Members) == 0 || !domain.ValidReasonCode(command.ReasonCode) {
		return fmt.Errorf("invalid split command")
	}
	seen := make(map[int64]struct{}, len(command.Members))
	for _, member := range command.Members {
		if member.ContentID <= 0 || member.ExpectedVersion <= 0 {
			return fmt.Errorf("invalid split member")
		}
		if _, ok := seen[member.ContentID]; ok {
			return fmt.Errorf("duplicate split member")
		}
		seen[member.ContentID] = struct{}{}
	}
	return nil
}

type MemberLockCommand struct {
	EventID, ContentID, ExpectedVersion int64
	Locked                              bool
	ActorUserID                         *int64
	ReasonCode                          string
}

func (command MemberLockCommand) Validate() error {
	if command.EventID <= 0 || command.ContentID <= 0 || command.ExpectedVersion <= 0 || !domain.ValidReasonCode(command.ReasonCode) {
		return fmt.Errorf("invalid member lock command")
	}
	return nil
}

type GovernanceRepository interface {
	Merge(context.Context, MergeCommand) (domain.Event, error)
	Split(context.Context, SplitCommand) (domain.Event, error)
	SetMemberLock(context.Context, MemberLockCommand) (domain.EventMember, error)
}

type GovernanceService struct {
	repository GovernanceRepository
}

func NewGovernanceService(repository GovernanceRepository) *GovernanceService {
	return &GovernanceService{repository: repository}
}

func (service *GovernanceService) Merge(ctx context.Context, command MergeCommand) (domain.Event, error) {
	if service == nil || service.repository == nil {
		return domain.Event{}, fmt.Errorf("%w: event governance repository is required", sharedrepository.ErrUnavailable)
	}
	if err := command.Validate(); err != nil {
		return domain.Event{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return service.repository.Merge(ctx, command)
}

func (service *GovernanceService) Split(ctx context.Context, command SplitCommand) (domain.Event, error) {
	if service == nil || service.repository == nil {
		return domain.Event{}, fmt.Errorf("%w: event governance repository is required", sharedrepository.ErrUnavailable)
	}
	if err := command.Validate(); err != nil {
		return domain.Event{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return service.repository.Split(ctx, command)
}

func (service *GovernanceService) SetMemberLock(ctx context.Context, command MemberLockCommand) (domain.EventMember, error) {
	if service == nil || service.repository == nil {
		return domain.EventMember{}, fmt.Errorf("%w: event governance repository is required", sharedrepository.ErrUnavailable)
	}
	if err := command.Validate(); err != nil {
		return domain.EventMember{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return service.repository.SetMemberLock(ctx, command)
}
