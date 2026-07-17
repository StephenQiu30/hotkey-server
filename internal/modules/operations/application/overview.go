package application

import (
	"context"
	"fmt"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type OverviewStore interface {
	RuntimeOverview(context.Context) (operationsdomain.RuntimeOverview, error)
}

type OverviewService struct{ store OverviewStore }

func NewOverviewService(store OverviewStore) (*OverviewService, error) {
	if store == nil {
		return nil, fmt.Errorf("overview store is required")
	}
	return &OverviewService{store: store}, nil
}

func (service *OverviewService) Get(ctx context.Context) (operationsdomain.RuntimeOverview, error) {
	if service == nil || service.store == nil {
		return operationsdomain.RuntimeOverview{}, sharedrepository.ErrUnavailable
	}
	return service.store.RuntimeOverview(ctx)
}
