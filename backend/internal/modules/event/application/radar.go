package application

import (
	"context"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type RadarRepository interface {
	ListRadar(context.Context, domain.RadarQuery) (domain.RadarPage, error)
}

type RadarService struct {
	repository RadarRepository
}

func NewRadarService(repository RadarRepository) *RadarService {
	return &RadarService{repository: repository}
}

func (service *RadarService) List(ctx context.Context, query domain.RadarQuery) (domain.RadarPage, error) {
	if service == nil || service.repository == nil {
		return domain.RadarPage{}, fmt.Errorf("%w: Radar repository is unavailable", sharedrepository.ErrUnavailable)
	}
	if query.Window == "" {
		query.Window = domain.RadarWindow24Hours
	}
	if query.Sort == "" {
		query.Sort = domain.RadarSortMomentum
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.AsOf.IsZero() {
		query.AsOf = time.Now().UTC()
	} else {
		query.AsOf = query.AsOf.UTC()
	}
	if err := query.Validate(); err != nil {
		return domain.RadarPage{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	page, err := service.repository.ListRadar(ctx, query)
	if err != nil {
		return domain.RadarPage{}, err
	}
	if page.AsOf.IsZero() {
		page.AsOf = query.AsOf
	}
	if page.Items == nil {
		page.Items = []domain.RadarEvent{}
	}
	return page, nil
}
