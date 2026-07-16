package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

const collectionFetchLimit = 100

// CollectionDependencies are intentionally separate from the administrative
// Source Service dependencies. Collection runs do not need authorization or
// audit writes, but they do need a Source-owned connection lookup, durable
// collection repository and a fixed connector registry.
type CollectionDependencies struct {
	Runtime    *database.Runtime
	Sources    domain.SourceConnectionRepository
	Runs       domain.CollectionRepository
	Connectors domain.CollectionConnectorRegistry
	Now        func() time.Time
}

type CollectionService struct {
	runtime    *database.Runtime
	sources    domain.SourceConnectionRepository
	runs       domain.CollectionRepository
	connectors domain.CollectionConnectorRegistry
	now        func() time.Time
}

func NewCollectionService(dependencies CollectionDependencies) (*CollectionService, error) {
	if dependencies.Runtime == nil || dependencies.Sources == nil || dependencies.Runs == nil || dependencies.Connectors == nil {
		return nil, errors.New("collection application dependencies are required")
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	return &CollectionService{
		runtime: dependencies.Runtime, sources: dependencies.Sources, runs: dependencies.Runs,
		connectors: dependencies.Connectors, now: dependencies.Now,
	}, nil
}

// Collect creates or reuses the source/signature/window run, claims it before
// issuing external I/O, then returns to a single database transaction to
// persist captured facts and checkpoints. A reused run never triggers a
// second fetch.
func (service *CollectionService) Collect(ctx context.Context, request domain.CollectionRequest) (domain.CollectionRun, error) {
	if service == nil || service.runtime == nil {
		return domain.CollectionRun{}, errors.New("collection service is not initialized")
	}
	if err := request.Validate(); err != nil {
		return domain.CollectionRun{}, domain.NewCollectionError(domain.CollectionErrorPermanent, err)
	}
	run, created, err := service.runs.CreateOrReuseRun(ctx, request)
	if err != nil {
		return domain.CollectionRun{}, err
	}
	if !created {
		return run, nil
	}
	run, started, err := service.runs.StartRun(ctx, run.ID)
	if err != nil {
		return domain.CollectionRun{}, err
	}
	if !started {
		return run, nil
	}

	connection, err := service.sources.FindByID(ctx, request.SourceConnectionID)
	if err != nil {
		return service.fail(ctx, run, request.Targets, domain.FetchResult{}, domain.CollectionErrorPermanent, err)
	}
	if !connection.Enabled || connection.Deleted {
		return service.fail(ctx, run, request.Targets, domain.FetchResult{}, domain.CollectionErrorPermanent, errors.New("source connection is unavailable"))
	}
	connector, err := service.connectors.Resolve(ctx, *connection)
	if err != nil {
		return service.fail(ctx, run, request.Targets, domain.FetchResult{}, domain.CollectionErrorPermanent, err)
	}
	if err := connector.Validate(ctx, *connection); err != nil {
		return service.fail(ctx, run, request.Targets, domain.FetchResult{}, domain.ClassifyCollectionError(err), err)
	}
	result, fetchErr := connector.Fetch(ctx, domain.FetchRequest{
		CollectionRunID: run.ID, SourceConnectionID: run.SourceConnectionID, QuerySignature: run.QuerySignature,
		Query: request.Query, Languages: append([]string(nil), request.Languages...), Regions: append([]string(nil), request.Regions...),
		WindowStart: run.WindowStart, WindowEnd: run.WindowEnd, RequestCursor: run.RequestCursor, ETag: run.ETag,
		LastModified: run.LastModified, Limit: collectionFetchLimit,
	})
	if fetchErr != nil {
		return service.fail(ctx, run, request.Targets, result, domain.ClassifyCollectionError(fetchErr), fetchErr)
	}
	captures := make([]domain.CapturedItem, 0, len(result.Items))
	policy := capturePolicy(*connection)
	for _, item := range result.Items {
		captured, err := policy.Capture(item)
		if err != nil {
			return service.fail(ctx, run, request.Targets, result, domain.CollectionErrorPermanent, err)
		}
		captures = append(captures, captured)
	}
	completed, err := service.runs.PersistSuccess(ctx, domain.CollectionRunSuccess{
		RunID: run.ID, Targets: request.Targets, Items: captures, Result: result, CompletedAt: service.now().UTC(),
	})
	if err != nil {
		return service.fail(ctx, run, request.Targets, result, domain.CollectionErrorTemporary, err)
	}
	return completed, nil
}

func (service *CollectionService) fail(ctx context.Context, run domain.CollectionRun, targets []domain.PublishedCollectionTarget, result domain.FetchResult, kind domain.CollectionErrorKind, cause error) (domain.CollectionRun, error) {
	if !kind.Valid() {
		kind = domain.CollectionErrorPermanent
	}
	failed, persistErr := service.runs.PersistFailure(ctx, domain.CollectionRunFailure{
		RunID: run.ID, Targets: targets, Result: result, ErrorKind: kind, CompletedAt: service.now().UTC(),
	})
	if persistErr != nil {
		return domain.CollectionRun{}, fmt.Errorf("persist collection failure: %w", persistErr)
	}
	if cause == nil {
		cause = errors.New("collection failed")
	}
	return failed, domain.NewCollectionError(kind, cause)
}

func capturePolicy(connection domain.SourceConnection) domain.CapturePolicy {
	return domain.CapturePolicy{
		Version: domain.CapturedItemVersionV1, AllowBodyStorage: connection.Config.AllowBodyStorage,
		RawPayloadDisposition: domain.RawPayloadDiscarded,
	}
}
