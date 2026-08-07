package application

import (
	"context"
	"errors"
	"strconv"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// CollectionControlDependencies are separate from CollectionDependencies:
// these administrator operations never plan queries or issue Fetch calls.
type CollectionControlDependencies struct {
	Runtime    *database.Runtime
	Sources    domain.SourceConnectionRepository
	Runs       domain.CollectionRepository
	Connectors domain.CollectionConnectorRegistry
	Metrics    CollectionMetrics
	Retries    CollectionRetryActivator
	Manuals    ManualCollectionActivator
	Targets    domain.ManualCollectionTargetReader
	Now        func() time.Time
}

type CollectionRetryActivator interface {
	Reactivate(context.Context, domain.CollectionRunRetry) error
}

type ManualCollectionActivator interface {
	Enqueue(context.Context, domain.ManualCollectionCommand) (bool, error)
}

type CollectionControlService struct {
	runtime    *database.Runtime
	sources    domain.SourceConnectionRepository
	runs       domain.CollectionRepository
	connectors domain.CollectionConnectorRegistry
	metrics    CollectionMetrics
	retries    CollectionRetryActivator
	manuals    ManualCollectionActivator
	targets    domain.ManualCollectionTargetReader
	now        func() time.Time
}

func NewCollectionControlService(dependencies CollectionControlDependencies) (*CollectionControlService, error) {
	if dependencies.Runtime == nil || dependencies.Sources == nil || dependencies.Runs == nil || dependencies.Connectors == nil || dependencies.Retries == nil {
		return nil, errors.New("collection control dependencies are required")
	}
	if dependencies.Metrics == nil {
		dependencies.Metrics = noopCollectionMetrics{}
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	return &CollectionControlService{
		runtime: dependencies.Runtime, sources: dependencies.Sources, runs: dependencies.Runs,
		connectors: dependencies.Connectors, metrics: dependencies.Metrics, retries: dependencies.Retries,
		manuals: dependencies.Manuals, targets: dependencies.Targets, now: dependencies.Now,
	}, nil
}

type CollectionRunListInput struct {
	Subject identitydomain.Subject
	Query   domain.CollectionRunListQuery
}

type CollectionRunRetryInput struct {
	Subject identitydomain.Subject
	ID      int64
}

type ManualCollectionInput struct {
	Subject   identitydomain.Subject
	MonitorID int64
}

type SourceHealthInput struct {
	Subject identitydomain.Subject
	ID      int64
}

func (service *CollectionControlService) List(ctx context.Context, input CollectionRunListInput) (domain.CollectionRunPage, error) {
	if err := requireEditor(input.Subject); err != nil {
		return domain.CollectionRunPage{}, err
	}
	page, err := service.runs.ListRuns(ctx, input.Query)
	if err != nil {
		service.metrics.RecordCollectionOperation("list", "error")
		return domain.CollectionRunPage{}, collectionControlError(err)
	}
	service.metrics.RecordCollectionOperation("list", "success")
	return page, nil
}

// Manual submits one durable collect_source job per source/query group. It
// never calls a Connector and relies on the queue's unique key for atomic
// five-minute cooldown reuse.
func (service *CollectionControlService) Manual(ctx context.Context, input ManualCollectionInput) (domain.ManualCollectionSummary, error) {
	if err := requireEditor(input.Subject); err != nil {
		return domain.ManualCollectionSummary{}, err
	}
	if input.MonitorID <= 0 {
		return domain.ManualCollectionSummary{}, domain.InvalidCollectionRequest()
	}
	if service.manuals == nil || service.targets == nil {
		return domain.ManualCollectionSummary{}, sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	now := service.now().UTC()
	type groupKey struct {
		sourceID  int64
		signature string
	}
	type group struct {
		configVersionID int64
		interval        time.Duration
	}
	var summary domain.ManualCollectionSummary
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		lockKey := "hotkey.manual_collection:" + strconv.FormatInt(input.MonitorID, 10) + ":" + strconv.FormatInt(now.Truncate(5*time.Minute).Unix(), 10)
		if _, err := transaction.SQL.ExecContext(transactionCtx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			return err
		}
		targets, err := service.targets.ListForManualCollection(transactionCtx, input.MonitorID)
		if err != nil {
			return err
		}
		groups := make(map[groupKey]group, len(targets))
		for _, target := range targets {
			key := groupKey{sourceID: target.SourceConnectionID, signature: target.QuerySignature}
			candidate := group{configVersionID: target.MonitorConfigVersionID, interval: target.CollectionInterval}
			if current, ok := groups[key]; !ok || candidate.configVersionID < current.configVersionID {
				groups[key] = candidate
			}
		}
		summary.Requested = len(groups)
		summary.CooldownUntil = now.Truncate(5 * time.Minute).Add(5 * time.Minute)
		for key, item := range groups {
			created, err := service.manuals.Enqueue(transactionCtx, domain.ManualCollectionCommand{
				SourceConnectionID: key.sourceID, ConfigVersionID: item.configVersionID, QuerySignature: key.signature,
				WindowStart: now.Add(-item.interval), WindowEnd: now, ScheduledAt: now,
			})
			if err != nil {
				return err
			}
			if created {
				summary.Created++
			} else {
				summary.Reused++
			}
		}
		return nil
	})
	if err != nil {
		service.metrics.RecordCollectionOperation("manual", "error")
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return domain.ManualCollectionSummary{}, domain.CollectionRunConflict()
		}
		return domain.ManualCollectionSummary{}, collectionControlError(err)
	}
	service.metrics.RecordCollectionOperation("manual", "success")
	return summary, nil
}

// Retry atomically restores the failed window and reactivates its original
// durable job. The request does not call Fetch or create a new queue record.
func (service *CollectionControlService) Retry(ctx context.Context, input CollectionRunRetryInput) (domain.CollectionRunSummary, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return domain.CollectionRunSummary{}, err
	}
	if input.ID <= 0 {
		return domain.CollectionRunSummary{}, domain.InvalidCollectionRequest()
	}
	var summary domain.CollectionRunSummary
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(transactionCtx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "hotkey.monitor_source_configuration"); err != nil {
			return err
		}
		var err error
		summary, err = service.runs.RetryRunWithHook(transactionCtx, input.ID, service.retries.Reactivate)
		return err
	})
	if err != nil {
		service.metrics.RecordCollectionOperation("retry", "error")
		return domain.CollectionRunSummary{}, collectionControlError(err)
	}
	service.metrics.RecordCollectionOperation("retry", "success")
	return summary, nil
}

// Health probes the immutable connection snapshot outside a transaction and
// only then records its safe status. A concurrent source edit produces a
// stable conflict rather than overwriting newer connection facts.
func (service *CollectionControlService) Health(ctx context.Context, input SourceHealthInput) (domain.SourceHealth, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return domain.SourceHealth{}, err
	}
	if input.ID <= 0 {
		return domain.SourceHealth{}, domain.InvalidCollectionRequest()
	}
	connection, err := service.sources.FindByID(ctx, input.ID)
	if err != nil {
		service.metrics.RecordCollectionOperation("health", "error")
		return domain.SourceHealth{}, sourceHealthReadError(err)
	}
	if connection.Deleted {
		service.metrics.RecordCollectionOperation("health", "error")
		return domain.SourceHealth{}, domain.SourceConnectionUnavailable()
	}

	probe := domain.HealthResult{CheckedAt: service.now().UTC(), ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "connector_unavailable"}
	connector, resolveErr := service.connectors.Resolve(ctx, *connection)
	if resolveErr == nil {
		probe = connector.Health(ctx, *connection)
	}
	if probe.CheckedAt.IsZero() {
		probe.CheckedAt = service.now().UTC()
	}
	result := domain.SourceHealth{Healthy: probe.Healthy, CheckedAt: probe.CheckedAt.UTC()}
	if !probe.Healthy {
		result.ErrorCode = safeHealthCode(probe.DiagnosticCode)
	}
	if err := service.persistHealth(ctx, *connection, probe); err != nil {
		service.metrics.RecordCollectionOperation("health", "error")
		return domain.SourceHealth{}, err
	}
	if result.Healthy {
		service.metrics.RecordCollectionOperation("health", "healthy")
	} else {
		service.metrics.RecordCollectionOperation("health", "unhealthy")
	}
	return result, nil
}

func (service *CollectionControlService) persistHealth(ctx context.Context, observed domain.SourceConnection, probe domain.HealthResult) error {
	if service == nil || service.runtime == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	return service.runtime.WithinTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		current, err := service.sources.LockByID(ctx, observed.ID)
		if err != nil {
			return sourceHealthReadError(err)
		}
		if current.Version != observed.Version || current.Deleted {
			return domain.SourceConnectionUnavailable()
		}
		next := *current
		next.HealthStatus = healthStatus(probe)
		if err := service.sources.Update(ctx, &next); err != nil {
			return sourceHealthWriteError(err)
		}
		return nil
	})
}

func healthStatus(probe domain.HealthResult) domain.HealthStatus {
	if probe.Healthy {
		return domain.HealthStatusHealthy
	}
	switch probe.ErrorKind {
	case domain.CollectionErrorAuthentication, domain.CollectionErrorPermanent:
		return domain.HealthStatusUnavailable
	default:
		return domain.HealthStatusDegraded
	}
}

func safeHealthCode(value string) string {
	switch value {
	case "invalid_source_connection", "request_failed", "upstream_status", "connector_unavailable", "destination_not_permitted", "credential_unavailable":
		return value
	default:
		return "probe_failed"
	}
}

func collectionControlError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrNotFound):
		return domain.CollectionRunNotFound()
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrConstraint):
		return domain.CollectionRunConflict()
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return domain.InvalidCollectionRequest()
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	default:
		return err
	}
}

func sourceHealthReadError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	if errors.Is(err, sharedrepository.ErrUnavailable) {
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	return domain.SourceConnectionUnavailable()
}

func sourceHealthWriteError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	if errors.Is(err, sharedrepository.ErrUnavailable) {
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	return domain.SourceConnectionUnavailable()
}
