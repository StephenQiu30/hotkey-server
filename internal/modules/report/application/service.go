package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// Service owns the report-facing application contract. Preview is read-only;
// only Publish changes a draft, and the repository makes published rows
// immutable afterwards.
type Service struct {
	store         Store
	builder       *Builder
	events        EventReader
	subscriptions SubscriptionReader
	delivery      DeliveryPlanner
	publish       Publisher
}

type Publisher interface {
	Publish(context.Context, domain.Report) error
}

type EventReader interface {
	List(context.Context, eventdomain.EventListQuery) (eventdomain.EventPage, error)
}

// AutomationSubscription is the minimum delivery configuration needed to
// produce one monitor-scoped report. It deliberately excludes user identity
// and secrets from the report module.
type AutomationSubscription struct {
	ID, Version int64
	MonitorID   *int64
	ReportType  domain.ReportType
	Channel     string
	Timezone    string
	Enabled     bool
}

type SubscriptionReader interface {
	GetEnabledSubscription(context.Context, int64) (AutomationSubscription, error)
}

type AutomaticStore interface {
	FindByPeriod(context.Context, domain.ReportType, *int64, time.Time, time.Time) (domain.Report, error)
	Create(context.Context, domain.Report) (domain.Report, error)
}

// DeliveryPlanner creates idempotent delivery rows and queues their delivery
// jobs after a report has become immutable and visible.
type DeliveryPlanner interface {
	Schedule(context.Context, domain.Report) error
}

type BuildInput struct {
	ID        int64
	Type      domain.ReportType
	At        time.Time
	Timezone  string
	MonitorID *int64
	Events    []EventSnapshot
}

func NewService(store Store, readers ...EventReader) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("report store is required")
	}
	service := &Service{store: store, builder: NewBuilder()}
	if len(readers) > 0 {
		service.events = readers[0]
	}
	return service, nil
}

func (service *Service) SetPublisher(publisher Publisher) { service.publish = publisher }

func (service *Service) SetSubscriptionReader(reader SubscriptionReader) {
	service.subscriptions = reader
}

func (service *Service) SetDeliveryPlanner(planner DeliveryPlanner) { service.delivery = planner }

// BuildByID is the durable queue entry point. It rereads the current report
// definition and a bounded event page; the queue payload contains only ID.
func (service *Service) BuildByID(ctx context.Context, reportID int64) (domain.Report, error) {
	if service == nil || service.events == nil || reportID <= 0 {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	current, err := service.Get(ctx, reportID)
	if err != nil {
		return domain.Report{}, err
	}
	page, err := service.events.List(ctx, eventdomain.EventListQuery{Limit: 100, MonitorID: current.MonitorID})
	if err != nil {
		return domain.Report{}, err
	}
	events := make([]EventSnapshot, 0, len(page.Items))
	for _, event := range page.Items {
		events = append(events, EventSnapshot{EventID: event.ID, Title: event.TitleZH, Summary: event.Summary, HeatScore: event.HeatScore})
	}
	timezone := "UTC"
	if current.Period.Location != nil {
		timezone = current.Period.Location.String()
	}
	return service.Build(ctx, BuildInput{ID: reportID, Type: current.Type, At: time.Now().UTC(), Timezone: timezone, MonitorID: current.MonitorID, Events: events})
}

// BuildAndPublishForSubscription is the unattended report path. A
// subscription is the only schedule input; the service derives the calendar
// period, monitor scope, report snapshot, publication and delivery trigger.
func (service *Service) BuildAndPublishForSubscription(ctx context.Context, subscriptionID int64) (domain.Report, error) {
	if service == nil || service.events == nil || service.subscriptions == nil || subscriptionID <= 0 {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	automatic, ok := service.store.(AutomaticStore)
	if !ok {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	subscription, err := service.subscriptions.GetEnabledSubscription(ctx, subscriptionID)
	if err != nil {
		return domain.Report{}, err
	}
	if !subscription.Enabled || subscription.ID <= 0 || subscription.Version <= 0 || subscription.ReportType == "" {
		return domain.Report{}, sharedrepository.ErrConflict
	}
	timezone := subscription.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return domain.Report{}, fmt.Errorf("invalid subscription timezone: %w", err)
	}
	at := time.Now().UTC()
	period, err := domain.PeriodFor(at, subscription.ReportType, location)
	if err != nil {
		return domain.Report{}, err
	}
	report, err := automatic.FindByPeriod(ctx, subscription.ReportType, subscription.MonitorID, period.Start, period.End)
	if err != nil && !errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.Report{}, err
	}
	if errors.Is(err, sharedrepository.ErrNotFound) {
		draft, buildErr := service.builder.Build(1, subscription.ReportType, at, location, nil)
		if buildErr != nil {
			return domain.Report{}, buildErr
		}
		draft.MonitorID = subscription.MonitorID
		report, err = automatic.Create(ctx, draft)
		if err != nil {
			return domain.Report{}, err
		}
	}
	if report.Status == domain.ReportPublished {
		if service.delivery != nil {
			if err := service.delivery.Schedule(ctx, report); err != nil {
				return domain.Report{}, err
			}
		}
		return report, nil
	}
	page, err := service.events.List(ctx, eventdomain.EventListQuery{Limit: 100, MonitorID: subscription.MonitorID})
	if err != nil {
		return domain.Report{}, err
	}
	events := make([]EventSnapshot, 0, len(page.Items))
	for _, event := range page.Items {
		events = append(events, EventSnapshot{EventID: event.ID, Title: event.TitleZH, Summary: event.Summary, HeatScore: event.HeatScore})
	}
	if _, err := service.Build(ctx, BuildInput{ID: report.ID, Type: report.Type, At: at, Timezone: timezone, MonitorID: report.MonitorID, Events: events}); err != nil {
		return domain.Report{}, err
	}
	return service.Publish(ctx, report.ID)
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
	if service.publish != nil {
		if err := service.publish.Publish(ctx, published); err != nil {
			return domain.Report{}, err
		}
	}
	if err := service.store.Save(ctx, published); err != nil {
		return domain.Report{}, err
	}
	if service.delivery != nil {
		if err := service.delivery.Schedule(ctx, published); err != nil {
			return domain.Report{}, fmt.Errorf("schedule report delivery: %w", err)
		}
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
