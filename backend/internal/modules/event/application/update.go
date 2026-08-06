package application

import (
	"context"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type UpdateRepository interface {
	PreviousHeatSnapshot(context.Context, int64, int, time.Time) (*domain.HeatResult, error)
	AppendUpdate(context.Context, domain.EventUpdateCandidate) (*domain.EventUpdate, bool, error)
	ListUpdates(context.Context, domain.EventUpdateListQuery) (domain.EventUpdatePage, error)
}

type UpdateService struct{ repository UpdateRepository }

func NewUpdateService(repository UpdateRepository) *UpdateService {
	return &UpdateService{repository: repository}
}

func (service *UpdateService) Record(ctx context.Context, current domain.HeatResult) (*domain.EventUpdate, bool, error) {
	if service == nil || service.repository == nil {
		return nil, false, fmt.Errorf("%w: event update repository is required", sharedrepository.ErrUnavailable)
	}
	if _, err := domain.EventUpdateIdempotencyKey(current); err != nil {
		return nil, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	previous, err := service.repository.PreviousHeatSnapshot(ctx, current.EventID, 24, current.WindowEnd.UTC())
	if err != nil {
		return nil, false, err
	}
	candidate, err := domain.DetectEventUpdate(previous, current)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if candidate == nil {
		return nil, false, nil
	}
	return service.repository.AppendUpdate(ctx, *candidate)
}

func (service *UpdateService) List(ctx context.Context, eventID int64, limit int, cursor int64) (domain.EventUpdatePage, error) {
	if service == nil || service.repository == nil {
		return domain.EventUpdatePage{}, fmt.Errorf("%w: event update repository is required", sharedrepository.ErrUnavailable)
	}
	if eventID <= 0 || limit < 1 || limit > 100 || cursor < 0 {
		return domain.EventUpdatePage{}, fmt.Errorf("%w: invalid event update list query", sharedrepository.ErrInvalidInput)
	}
	return service.repository.ListUpdates(ctx, domain.EventUpdateListQuery{EventID: eventID, Limit: limit, Cursor: cursor})
}
