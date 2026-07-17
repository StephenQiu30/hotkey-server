package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type EventStore interface {
	Get(context.Context, int64) (domain.Event, error)
	Save(context.Context, domain.Event, int64, domain.GovernanceAudit) error
}

type LifecycleInput struct {
	EventID         int64
	ExpectedVersion int64
	To              domain.LifecycleStatus
	ReasonCode      string
	ActorUserID     *int64
}

type LifecycleService struct {
	store     EventStore
	recompute MetricRecomputer
}

func NewLifecycleService(store EventStore, recomputers ...MetricRecomputer) *LifecycleService {
	service := &LifecycleService{store: store}
	if len(recomputers) > 0 {
		service.recompute = recomputers[0]
	}
	return service
}

func (service *LifecycleService) Transition(ctx context.Context, input LifecycleInput) (domain.Event, error) {
	if service == nil || service.store == nil || input.EventID <= 0 || input.ExpectedVersion <= 0 || !input.To.Valid() || !domain.ValidReasonCode(input.ReasonCode) {
		return domain.Event{}, fmt.Errorf("%w: invalid lifecycle input", sharedrepository.ErrInvalidInput)
	}
	event, err := service.store.Get(ctx, input.EventID)
	if err != nil {
		return domain.Event{}, err
	}
	if event.Version != input.ExpectedVersion {
		return domain.Event{}, fmt.Errorf("%w: event version conflict", sharedrepository.ErrConflict)
	}
	if event.ManualLocked && input.To != event.LifecycleStatus {
		return domain.Event{}, fmt.Errorf("%w: event is manually locked", sharedrepository.ErrConflict)
	}
	if event.LifecycleStatus == input.To {
		return event, nil
	}
	if !domain.CanTransition(event.LifecycleStatus, input.To) {
		return domain.Event{}, fmt.Errorf("%w: invalid lifecycle transition %s -> %s", sharedrepository.ErrConflict, event.LifecycleStatus, input.To)
	}
	from := event.LifecycleStatus
	event.LifecycleStatus = input.To
	if input.To != domain.LifecycleMerged {
		event.MergedIntoID = nil
	}
	audit := domain.GovernanceAudit{EventID: event.ID, Action: domain.AuditLifecycleTransition, ActorUserID: input.ActorUserID, ReasonCode: input.ReasonCode, FromStatus: &from, ToStatus: &input.To, ExpectedVersion: &input.ExpectedVersion}
	if err := service.store.Save(ctx, event, input.ExpectedVersion, audit); err != nil {
		return domain.Event{}, err
	}
	if err := recomputeCurrentEventMetrics(ctx, service.recompute, event.ID); err != nil {
		return domain.Event{}, err
	}
	return event, nil
}
