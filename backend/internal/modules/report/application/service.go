package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// Service owns the report-facing application contract. Preview is read-only;
// only Publish changes a draft, and the repository makes published rows
// immutable afterwards.
type Service struct {
	store         Store
	builder       *Builder
	snapshots     SnapshotReader
	subscriptions SubscriptionReader
	delivery      DeliveryPlanner
	archive       ArchivePlanner
	transactions  TransactionRunner
}

type SnapshotReader interface {
	ListForPeriod(context.Context, *int64, time.Time, time.Time, int) ([]EventSnapshot, error)
}

// AutomationSubscription is the minimum delivery configuration needed to
// produce one monitor-scoped report. It deliberately excludes user identity
// and secrets from the report module.
type AutomationSubscription struct {
	ID, Version, UserID int64
	MonitorID           *int64
	ReportType          domain.ReportType
	Channel             string
	Timezone            string
	Enabled             bool
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

type ArchivePlanner interface {
	Prepare(context.Context, domain.Report) error
}

type TransactionRunner interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

type BuildInput struct {
	ID        int64
	Type      domain.ReportType
	At        time.Time
	Timezone  string
	MonitorID *int64
	Events    []EventSnapshot
	ActorID   *int64
}

type CreateInput struct {
	Type      domain.ReportType
	At        time.Time
	Timezone  string
	MonitorID *int64
	ActorID   int64
}

func NewService(store Store, readers ...SnapshotReader) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("report store is required")
	}
	service := &Service{store: store, builder: NewBuilder()}
	if len(readers) > 0 {
		service.snapshots = readers[0]
	}
	if transactions, ok := store.(TransactionRunner); ok {
		service.transactions = transactions
	}
	return service, nil
}

func (service *Service) SetSubscriptionReader(reader SubscriptionReader) {
	service.subscriptions = reader
}

func (service *Service) SetDeliveryPlanner(planner DeliveryPlanner) { service.delivery = planner }
func (service *Service) SetArchivePlanner(planner ArchivePlanner)   { service.archive = planner }

// BuildByID is the durable queue entry point. It rereads the current report
// definition and a bounded event page; the queue payload contains only ID.
func (service *Service) BuildByID(ctx context.Context, reportID int64) (domain.Report, error) {
	if service == nil || service.snapshots == nil || reportID <= 0 {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	current, err := service.Get(ctx, reportID)
	if err != nil {
		return domain.Report{}, err
	}
	events, err := service.snapshots.ListForPeriod(ctx, current.MonitorID, current.Period.Start, current.Period.End, 100)
	if err != nil {
		return domain.Report{}, err
	}
	timezone := "UTC"
	if current.Period.Location != nil {
		timezone = current.Period.Location.String()
	}
	return service.Build(ctx, BuildInput{ID: reportID, Type: current.Type, At: current.Period.Start, Timezone: timezone, MonitorID: current.MonitorID, Events: events, ActorID: current.UpdatedBy})
}

// CreateDraft creates at most one draft for a type, monitor and calendar
// period, then refreshes its immutable EventUpdate candidate snapshots.
func (service *Service) CreateDraft(ctx context.Context, input CreateInput) (domain.Report, error) {
	if service == nil || service.snapshots == nil || input.ActorID <= 0 || input.At.IsZero() {
		return domain.Report{}, sharedrepository.ErrInvalidInput
	}
	location, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return domain.Report{}, fmt.Errorf("invalid report timezone: %w", err)
	}
	period, err := domain.PeriodFor(input.At, input.Type, location)
	if err != nil {
		return domain.Report{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	automatic, ok := service.store.(AutomaticStore)
	if !ok {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	report, err := automatic.FindByPeriod(ctx, input.Type, input.MonitorID, period.Start, period.End)
	if err != nil && !errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.Report{}, err
	}
	actor := input.ActorID
	if errors.Is(err, sharedrepository.ErrNotFound) {
		draft, buildErr := service.builder.Build(1, input.Type, input.At, location, nil)
		if buildErr != nil {
			return domain.Report{}, buildErr
		}
		draft.MonitorID, draft.CreatedBy, draft.UpdatedBy = input.MonitorID, &actor, &actor
		report, err = automatic.Create(ctx, draft)
		if err != nil {
			return domain.Report{}, err
		}
	}
	if report.Status != domain.ReportDraft {
		return domain.Report{}, sharedrepository.ErrImmutable
	}
	events, err := service.snapshots.ListForPeriod(ctx, input.MonitorID, period.Start, period.End, 100)
	if err != nil {
		return domain.Report{}, err
	}
	return service.Build(ctx, BuildInput{ID: report.ID, Type: input.Type, At: input.At, Timezone: input.Timezone, MonitorID: input.MonitorID, Events: events, ActorID: &actor})
}

// BuildAndPublishForSubscription is the unattended report path. A
// subscription is the only schedule input; the service derives the calendar
// period, monitor scope, report snapshot, publication and delivery trigger.
func (service *Service) BuildAndPublishForSubscription(ctx context.Context, subscriptionID int64) (domain.Report, error) {
	if service == nil || service.snapshots == nil || service.subscriptions == nil || subscriptionID <= 0 {
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
		if subscription.UserID > 0 {
			draft.CreatedBy, draft.UpdatedBy = &subscription.UserID, &subscription.UserID
		}
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
	events, err := service.snapshots.ListForPeriod(ctx, subscription.MonitorID, period.Start, period.End, 100)
	if err != nil {
		return domain.Report{}, err
	}
	var actor *int64
	if subscription.UserID > 0 {
		actor = &subscription.UserID
	}
	if _, err := service.Build(ctx, BuildInput{ID: report.ID, Type: report.Type, At: at, Timezone: timezone, MonitorID: report.MonitorID, Events: events, ActorID: actor}); err != nil {
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
	return service.PublishAs(ctx, reportID, 0)
}

func (service *Service) PublishAs(ctx context.Context, reportID, actorID int64) (domain.Report, error) {
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
	if actorID > 0 {
		published.UpdatedBy = &actorID
	}
	commit := func(transactionCtx context.Context) error {
		if err := service.store.ValidatePublication(transactionCtx, report); err != nil {
			return err
		}
		if err := service.store.Save(transactionCtx, published); err != nil {
			return err
		}
		if service.delivery != nil {
			if err := service.delivery.Schedule(transactionCtx, published); err != nil {
				return fmt.Errorf("schedule report delivery: %w", err)
			}
		}
		if service.archive != nil {
			if err := service.archive.Prepare(transactionCtx, published); err != nil {
				return fmt.Errorf("prepare report archive: %w", err)
			}
		}
		return nil
	}
	if service.transactions != nil {
		err = service.transactions.WithinTransaction(ctx, commit)
	} else {
		err = commit(ctx)
	}
	if err != nil {
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
	report.CreatedBy, report.UpdatedBy = input.ActorID, input.ActorID
	if current, getErr := service.store.Get(ctx, input.ID); getErr == nil {
		report.CreatedBy = current.CreatedBy
		if input.ActorID == nil {
			report.UpdatedBy = current.UpdatedBy
		}
	}
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
