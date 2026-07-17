package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// Service owns the report-facing application contract. Preview is read-only;
// only Publish changes a draft, and the repository makes published rows
// immutable afterwards.
type Service struct {
	store   Store
	builder *Builder
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("report store is required")
	}
	return &Service{store: store, builder: NewBuilder()}, nil
}

func (service *Service) List(ctx context.Context, query domain.ListQuery) (domain.Page, error) {
	if service == nil || service.store == nil {
		return domain.Page{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.Page{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return service.store.List(ctx, query)
}

func (service *Service) Get(ctx context.Context, reportID int64) (domain.Report, error) {
	if service == nil || service.store == nil || reportID <= 0 {
		return domain.Report{}, sharedrepository.ErrInvalidInput
	}
	return service.store.Get(ctx, reportID)
}

func (service *Service) Preview(ctx context.Context, reportID int64) (domain.Report, error) {
	return service.Get(ctx, reportID)
}

func (service *Service) Publish(ctx context.Context, reportID int64) (domain.Report, error) {
	report, err := service.Get(ctx, reportID)
	if err != nil {
		return domain.Report{}, err
	}
	if report.Status != domain.ReportDraft {
		return domain.Report{}, sharedrepository.ErrImmutable
	}
	published, err := service.builder.Publish(report)
	if err != nil {
		return domain.Report{}, err
	}
	if err := service.store.Save(ctx, published); err != nil {
		return domain.Report{}, err
	}
	return service.store.Get(ctx, reportID)
}
