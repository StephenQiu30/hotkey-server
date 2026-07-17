package application

import (
	"context"
	"fmt"
	"time"

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

type BuildInput struct {
	ID        int64
	Type      domain.ReportType
	At        time.Time
	Timezone  string
	MonitorID *int64
	Events    []EventSnapshot
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

// Build creates or replaces only a draft for the deterministic report key.
// EventSnapshot values are copied into report_items, so subsequent Event or
// heat updates cannot mutate a published report.
func (service *Service) Build(ctx context.Context, input BuildInput) (domain.Report, error) {
	if service == nil || service.store == nil || input.ID <= 0 || input.At.IsZero() {
		return domain.Report{}, sharedrepository.ErrInvalidInput
	}
	location, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return domain.Report{}, fmt.Errorf("invalid report timezone: %w", err)
	}
	report, err := service.builder.Build(input.ID, input.Type, input.At, location, input.Events)
	if err != nil {
		return domain.Report{}, err
	}
	report.MonitorID = input.MonitorID
	report.Summary = fallbackSummary(report.Items)
	if err := service.store.Save(ctx, report); err != nil {
		return domain.Report{}, err
	}
	return service.store.Get(ctx, report.ID)
}

func fallbackSummary(items []domain.Item) string {
	if len(items) == 0 {
		return "No events matched the requested period."
	}
	return fmt.Sprintf("%d frozen event snapshots selected for this report.", len(items))
}
