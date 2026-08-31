package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// Service owns the report-facing application contract. Preview is read-only;
// submission freezes draft content and only an Editor/Admin approval can make
// the revision published and immutable.
type Service struct {
	store         Store
	builder       *Builder
	snapshots     SnapshotReader
	subscriptions SubscriptionReader
	delivery      DeliveryPlanner
	archive       ArchivePlanner
	transactions  TransactionRunner
	securityAudit ContentSecurityAuditWriter
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

type RevisionLifecycleStore interface {
	Transition(context.Context, domain.RevisionTransition) (domain.Report, error)
}

type RevisionLifecycleInput struct {
	Subject         identitydomain.Subject
	ReportID        int64
	ExpectedVersion int64
	ReasonCode      string
}

var reportReasonCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,63}$`)

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
	Subject   identitydomain.Subject
	Type      domain.ReportType
	At        time.Time
	Timezone  string
	MonitorID *int64
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
func (service *Service) WithContentSecurityAudit(writer ContentSecurityAuditWriter) *Service {
	service.securityAudit = writer
	return service
}

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

func (service *Service) BuildByIDAs(ctx context.Context, subject identitydomain.Subject, reportID int64) (domain.Report, error) {
	if err := requireReportContributor(subject); err != nil {
		return domain.Report{}, err
	}
	return service.BuildByID(ctx, reportID)
}

// CreateDraft creates at most one draft for a type, monitor and calendar
// period, then refreshes its immutable EventUpdate candidate snapshots.
func (service *Service) CreateDraft(ctx context.Context, input CreateInput) (domain.Report, error) {
	if err := requireReportContributor(input.Subject); err != nil {
		return domain.Report{}, err
	}
	if service == nil || service.snapshots == nil || input.At.IsZero() {
		return domain.Report{}, sharedrepository.ErrInvalidInput
	}
	location, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return domain.Report{}, fmt.Errorf("invalid report timezone: %w", err)
	}
	period, err := domain.PeriodFor(input.At, input.Type, location)
	if err != nil {
		return domain.Report{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	automatic, ok := service.store.(AutomaticStore)
	if !ok {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	report, err := automatic.FindByPeriod(ctx, input.Type, input.MonitorID, period.Start, period.End)
	if err != nil && !errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.Report{}, err
	}
	actor := input.Subject.UserID
	if errors.Is(err, sharedrepository.ErrNotFound) {
		draft, buildErr := service.builder.Build(1, input.Type, input.At, location, nil)
		if buildErr != nil {
			return domain.Report{}, buildErr
		}
		draft.MonitorID, draft.CreatedBy, draft.UpdatedBy = input.MonitorID, &actor, &actor
		draft.InputSnapshotHash = domain.ComputeInputSnapshotHash(draft)
		report, err = automatic.Create(ctx, draft)
		if err != nil {
			return domain.Report{}, err
		}
	}
	if report.Status == domain.ReportPublished || report.Status == domain.ReportRejected {
		next, buildErr := service.builder.Build(1, input.Type, input.At, location, nil)
		if buildErr != nil {
			return domain.Report{}, buildErr
		}
		next.VersionNo = report.VersionNo + 1
		next.MonitorID, next.CreatedBy, next.UpdatedBy = input.MonitorID, &actor, &actor
		next.InputSnapshotHash = domain.ComputeInputSnapshotHash(next)
		report, err = automatic.Create(ctx, next)
		if err != nil {
			return domain.Report{}, err
		}
	} else if report.Status != domain.ReportDraft {
		return domain.Report{}, sharedrepository.ErrConflict
	}
	events, err := service.snapshots.ListForPeriod(ctx, input.MonitorID, period.Start, period.End, 100)
	if err != nil {
		return domain.Report{}, err
	}
	return service.Build(ctx, BuildInput{ID: report.ID, Type: input.Type, At: input.At, Timezone: input.Timezone, MonitorID: input.MonitorID, Events: events, ActorID: &actor})
}

// BuildForSubscription is the unattended report path. It may build a draft,
// but it never grants itself Editor authority or approves a revision.
func (service *Service) BuildForSubscription(ctx context.Context, subscriptionID int64) (domain.Report, error) {
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
		draft.InputSnapshotHash = domain.ComputeInputSnapshotHash(draft)
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
	if report.Status == domain.ReportPendingApproval || report.Status == domain.ReportRejected {
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
	return service.Build(ctx, BuildInput{ID: report.ID, Type: report.Type, At: at, Timezone: timezone, MonitorID: report.MonitorID, Events: events, ActorID: actor})
}

func (service *Service) List(ctx context.Context, query domain.ListQuery) (domain.Page, error) {
	if service == nil || service.store == nil {
		return domain.Page{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.Page{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
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

func (service *Service) SubmitForApproval(ctx context.Context, input RevisionLifecycleInput) (domain.Report, error) {
	if err := requireReportContributor(input.Subject); err != nil {
		return domain.Report{}, err
	}
	return service.transitionRevision(ctx, input, domain.ReportDraft, domain.ReportPendingApproval, true, false)
}

func (service *Service) ApproveRevision(ctx context.Context, input RevisionLifecycleInput) (domain.Report, error) {
	if err := requireReportEditor(input.Subject); err != nil {
		return domain.Report{}, err
	}
	return service.transitionRevision(ctx, input, domain.ReportPendingApproval, domain.ReportPublished, true, true)
}

func (service *Service) RejectRevision(ctx context.Context, input RevisionLifecycleInput) (domain.Report, error) {
	if err := requireReportEditor(input.Subject); err != nil {
		return domain.Report{}, err
	}
	if !reportReasonCodePattern.MatchString(input.ReasonCode) {
		return domain.Report{}, sharedrepository.ErrInvalidInput
	}
	return service.transitionRevision(ctx, input, domain.ReportPendingApproval, domain.ReportRejected, false, false)
}

func (service *Service) transitionRevision(ctx context.Context, input RevisionLifecycleInput, from, to domain.ReportStatus, validateEvidence, publish bool) (domain.Report, error) {
	if service == nil || service.store == nil || input.ReportID <= 0 || input.ExpectedVersion <= 0 || input.Subject.UserID <= 0 {
		return domain.Report{}, sharedrepository.ErrInvalidInput
	}
	lifecycle, ok := service.store.(RevisionLifecycleStore)
	if !ok {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	var transitioned domain.Report
	commit := func(transactionCtx context.Context) error {
		report, err := service.store.Get(transactionCtx, input.ReportID)
		if err != nil {
			return err
		}
		if report.Version != input.ExpectedVersion || report.Status != from {
			return sharedrepository.ErrConflict
		}
		if validateEvidence {
			if err := service.store.ValidatePublication(transactionCtx, report); err != nil {
				return err
			}
		}
		transitioned, err = lifecycle.Transition(transactionCtx, domain.RevisionTransition{ReportID: input.ReportID,
			ExpectedVersion: input.ExpectedVersion, ActorID: input.Subject.UserID, From: from, To: to, ReasonCode: input.ReasonCode})
		if err != nil {
			return err
		}
		if publish && service.delivery != nil {
			if err := service.delivery.Schedule(transactionCtx, transitioned); err != nil {
				return fmt.Errorf("schedule report delivery: %w", err)
			}
		}
		if publish && service.archive != nil {
			if err := service.archive.Prepare(transactionCtx, transitioned); err != nil {
				return fmt.Errorf("prepare report archive: %w", err)
			}
		}
		return nil
	}
	var err error
	if service.transactions != nil {
		err = service.transactions.WithinTransaction(ctx, commit)
	} else {
		err = commit(ctx)
	}
	if err != nil {
		if errors.Is(err, domain.ErrUnsafeContent) {
			if auditErr := writeContentSecurityRejection(ctx, service.securityAudit, input.Subject.UserID, input.ReportID); auditErr != nil {
				return domain.Report{}, errors.Join(err, fmt.Errorf("audit unsafe report content: %w", auditErr))
			}
		}
		return domain.Report{}, err
	}
	return transitioned, nil
}

func requireReportContributor(subject identitydomain.Subject) error {
	if subject.UserID <= 0 || !subject.Role.Valid() {
		return sharederrors.New(sharederrors.CodeUnauthenticated, 401, "")
	}
	if subject.Role != identitydomain.RoleAnalyst && subject.Role != identitydomain.RoleEditor && subject.Role != identitydomain.RoleAdmin {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	return nil
}

func requireReportEditor(subject identitydomain.Subject) error {
	if subject.UserID <= 0 || !subject.Role.Valid() {
		return sharederrors.New(sharederrors.CodeUnauthenticated, 401, "")
	}
	if subject.Role != identitydomain.RoleEditor && subject.Role != identitydomain.RoleAdmin {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	return nil
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
		if current.Status != domain.ReportDraft {
			return domain.Report{}, sharedrepository.ErrImmutable
		}
		report.Version = current.Version + 1
		report.VersionNo = current.VersionNo
		report.CreatedBy = current.CreatedBy
		if input.ActorID == nil {
			report.UpdatedBy = current.UpdatedBy
		}
	}
	report.Summary = fallbackSummary(report.Items)
	report.InputSnapshotHash = domain.ComputeInputSnapshotHash(report)
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
