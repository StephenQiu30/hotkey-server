package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type ReadRepository interface {
	List(context.Context, domain.EventListQuery) (domain.EventPage, error)
	Get(context.Context, int64) (domain.Event, error)
	ListMembers(context.Context, int64) (domain.EventMemberPage, error)
}

type ReadService struct {
	repository ReadRepository
}

func NewReadService(repository ReadRepository) *ReadService {
	return &ReadService{repository: repository}
}

func (service *ReadService) List(ctx context.Context, query domain.EventListQuery) (domain.EventPage, error) {
	if service == nil || service.repository == nil || query.Limit < 1 || query.Limit > 100 || query.Cursor < 0 || query.MonitorID != nil && *query.MonitorID <= 0 {
		return domain.EventPage{}, fmt.Errorf("%w: invalid event list query", sharedrepository.ErrInvalidInput)
	}
	return service.repository.List(ctx, query)
}

func (service *ReadService) Get(ctx context.Context, eventID int64) (domain.Event, error) {
	if service == nil || service.repository == nil || eventID <= 0 {
		return domain.Event{}, fmt.Errorf("%w: invalid event id", sharedrepository.ErrInvalidInput)
	}
	return service.repository.Get(ctx, eventID)
}

func (service *ReadService) ListMembers(ctx context.Context, eventID int64) (domain.EventMemberPage, error) {
	if service == nil || service.repository == nil || eventID <= 0 {
		return domain.EventMemberPage{}, fmt.Errorf("%w: invalid event id", sharedrepository.ErrInvalidInput)
	}
	return service.repository.ListMembers(ctx, eventID)
}
