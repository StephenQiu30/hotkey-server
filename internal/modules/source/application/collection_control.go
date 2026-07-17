package application

import (
	"context"
	"errors"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// CollectionControlDependencies are separate from CollectionDependencies:
// these administrator operations never plan queries or issue Fetch calls.
type CollectionControlDependencies struct {
	Runtime    *database.Runtime
	Sources    domain.SourceConnectionRepository
	Runs       domain.CollectionRepository
	Connectors domain.CollectionConnectorRegistry
	Metrics    CollectionMetrics
	Now        func() time.Time
}

type CollectionControlService struct {
	runtime    *database.Runtime
	sources    domain.SourceConnectionRepository
	runs       domain.CollectionRepository
	connectors domain.CollectionConnectorRegistry
	metrics    CollectionMetrics
	now        func() time.Time
}

func NewCollectionControlService(dependencies CollectionControlDependencies) (*CollectionControlService, error) {
	if dependencies.Runtime == nil || dependencies.Sources == nil || dependencies.Runs == nil || dependencies.Connectors == nil {
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
		connectors: dependencies.Connectors, metrics: dependencies.Metrics, now: dependencies.Now,
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

type SourceHealthInput struct {
	Subject identitydomain.Subject
	ID      int64
}

func (service *CollectionControlService) List(ctx context.Context, input CollectionRunListInput) (domain.CollectionRunPage, error) {
	if err := requireAdmin(input.Subject); err != nil {
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

// Retry is an explicit state command, not an execution shortcut. The durable
// run is requeued for the normal scheduler; this request neither calls Fetch
// nor creates a Cron/River job.
func (service *CollectionControlService) Retry(ctx context.Context, input CollectionRunRetryInput) (domain.CollectionRunSummary, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return domain.CollectionRunSummary{}, err
	}
	if input.ID <= 0 {
		return domain.CollectionRunSummary{}, domain.InvalidCollectionRequest()
	}
	summary, err := service.runs.RetryRun(ctx, input.ID)
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
	case "invalid_source_connection", "request_failed", "upstream_status", "connector_unavailable", "destination_not_permitted":
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
