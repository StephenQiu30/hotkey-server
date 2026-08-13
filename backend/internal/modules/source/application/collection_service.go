package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"go.uber.org/zap"
)

const (
	collectionFetchLimit      = 100
	collectionRunReclaimAfter = 5 * time.Minute
)

// CollectionDependencies are intentionally separate from the administrative
// Source Service dependencies. Collection runs do not need authorization or
// audit writes, but they do need a Source-owned connection lookup, durable
// collection repository and a fixed connector registry.
type CollectionDependencies struct {
	Runtime    *database.Runtime
	Sources    domain.SourceConnectionRepository
	Runs       domain.CollectionRepository
	Connectors domain.CollectionConnectorRegistry
	Evidence   CollectionEvidenceArchiver
	Now        func() time.Time

	// Logger is optionally injected for operational diagnostics. A nil logger
	// disables failure logging, so direct constructions in tests keep working.
	Logger *zap.Logger
}

type CollectionService struct {
	runtime    *database.Runtime
	sources    domain.SourceConnectionRepository
	runs       domain.CollectionRepository
	connectors domain.CollectionConnectorRegistry
	evidence   CollectionEvidenceArchiver
	now        func() time.Time
	logger     *zap.Logger
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
		connectors: dependencies.Connectors, evidence: dependencies.Evidence, now: dependencies.Now, logger: dependencies.Logger,
	}, nil
}

// Collect creates or reuses the source/signature/window run, claims it before
// issuing external I/O, then returns to a single database transaction to
// persist captured facts and checkpoints. A reused run never triggers a
// second fetch.
func (service *CollectionService) Collect(ctx context.Context, request domain.CollectionRequest) (domain.CollectionRun, error) {
	return service.collect(ctx, request, nil)
}

// CollectWithSuccessHook is the queue-aware entry point. When the Source
// repository supports the transaction hook, downstream enqueue happens in the
// same transaction as captured items and checkpoint advancement.
func (service *CollectionService) CollectWithSuccessHook(ctx context.Context, request domain.CollectionRequest, hook func(context.Context, int64) error) (domain.CollectionRun, error) {
	return service.collect(ctx, request, hook)
}

type CollectionRequestResolver func(context.Context) (domain.CollectionRequest, error)

// CollectResolvedWithSuccessHook keeps target rereading, request planning and
// run claiming under the same source/query advisory lock. External Fetch runs
// only after that transaction commits.
func (service *CollectionService) CollectResolvedWithSuccessHook(ctx context.Context, sourceConnectionID int64, querySignature string, resolve CollectionRequestResolver, hook func(context.Context, int64) error) (domain.CollectionRun, error) {
	if service == nil || service.runtime == nil || resolve == nil || sourceConnectionID <= 0 || querySignature == "" {
		return domain.CollectionRun{}, errors.New("collection service is not initialized")
	}
	var request domain.CollectionRequest
	var run domain.CollectionRun
	var started bool
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(transactionCtx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, domain.CollectionClaimKey(sourceConnectionID, querySignature)); err != nil {
			return err
		}
		var err error
		request, err = resolve(transactionCtx)
		if err != nil {
			return err
		}
		if err := request.Validate(); err != nil {
			return domain.NewCollectionError(domain.CollectionErrorPermanent, err)
		}
		if request.SourceConnectionID != sourceConnectionID || request.QuerySignature != querySignature {
			return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("resolved collection identity changed"))
		}
		run, _, err = service.runs.CreateOrReuseRun(transactionCtx, request)
		if err != nil {
			return err
		}
		run, started, err = service.runs.StartRun(transactionCtx, run.ID, service.now().UTC().Add(-collectionRunReclaimAfter))
		return err
	})
	if err != nil {
		return domain.CollectionRun{}, err
	}
	return service.execute(ctx, request, run, started, hook)
}

func (service *CollectionService) collect(ctx context.Context, request domain.CollectionRequest, successHook func(context.Context, int64) error) (domain.CollectionRun, error) {
	if service == nil || service.runtime == nil {
		return domain.CollectionRun{}, errors.New("collection service is not initialized")
	}
	if err := request.Validate(); err != nil {
		return domain.CollectionRun{}, domain.NewCollectionError(domain.CollectionErrorPermanent, err)
	}
	return service.CollectResolvedWithSuccessHook(ctx, request.SourceConnectionID, request.QuerySignature, func(context.Context) (domain.CollectionRequest, error) {
		return request, nil
	}, successHook)
}

func (service *CollectionService) execute(ctx context.Context, request domain.CollectionRequest, run domain.CollectionRun, started bool, successHook func(context.Context, int64) error) (domain.CollectionRun, error) {
	if !started {
		if run.Status == domain.CollectionRunSucceeded && successHook != nil {
			if writer, ok := service.runs.(interface {
				PersistSuccessWith(context.Context, domain.CollectionRunSuccess, func(context.Context, int64) error) (domain.CollectionRun, error)
			}); ok {
				if _, err := writer.PersistSuccessWith(ctx, domain.CollectionRunSuccess{RunID: run.ID, Targets: request.Targets, CompletedAt: service.now().UTC()}, successHook); err != nil {
					return domain.CollectionRun{}, err
				}
			} else if err := successHook(ctx, run.ID); err != nil {
				return domain.CollectionRun{}, err
			}
		}
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
	result.Items = filterCollectionItems(result.Items, request.Targets[0].Terms)
	if service.evidence != nil && len(result.Items) > 0 && len(result.Snapshots) > 0 {
		if _, err := service.evidence.ArchiveFetch(ctx, ArchiveCollectionEvidenceCommand{
			SourceConnectionID: run.SourceConnectionID,
			CollectionRunID:    run.ID,
			Fetch:              rawEvidenceFetchDTOFromEntity(result),
		}); err != nil {
			if errors.Is(err, domain.ErrRawArchiveNotAuthorized) {
				if service.logger != nil {
					service.logger.Info("raw evidence archive skipped by current rights policy",
						zap.Int64("source_connection_id", run.SourceConnectionID),
						zap.Int64("collection_run_id", run.ID),
					)
				}
			} else {
				return service.fail(ctx, run, request.Targets, result, domain.CollectionErrorTemporary, errors.New("raw evidence archive failed"))
			}
		}
	}
	captures := make([]domain.CapturedItem, 0, len(result.Items))
	policy := captureMetadataPolicy()
	for _, item := range result.Items {
		captured, err := policy.Capture(item)
		if err != nil {
			return service.fail(ctx, run, request.Targets, result, domain.CollectionErrorPermanent, err)
		}
		captures = append(captures, captured)
	}
	success := domain.CollectionRunSuccess{
		RunID: run.ID, Targets: request.Targets, Items: captures, Result: result, CompletedAt: service.now().UTC(),
	}
	var completed domain.CollectionRun
	if writer, ok := service.runs.(interface {
		PersistSuccessWith(context.Context, domain.CollectionRunSuccess, func(context.Context, int64) error) (domain.CollectionRun, error)
	}); ok {
		completed, err = writer.PersistSuccessWith(ctx, success, successHook)
	} else {
		completed, err = service.runs.PersistSuccess(ctx, success)
		if err == nil && successHook != nil {
			err = successHook(ctx, completed.ID)
		}
	}
	if err != nil {
		return service.fail(ctx, run, request.Targets, result, domain.CollectionErrorTemporary, err)
	}
	return completed, nil
}

func filterCollectionItems(items []domain.SourceItem, terms []domain.CollectionTerm) []domain.SourceItem {
	includes := []string{}
	excludes := []string{}
	for _, term := range terms {
		value := normalizedCollectionText(term.Value)
		if value == "" {
			continue
		}
		if term.Excluded {
			excludes = append(excludes, value)
		} else {
			includes = append(includes, value)
		}
	}
	filtered := make([]domain.SourceItem, 0, len(items))
	for _, item := range items {
		text := normalizedCollectionText(item.Title + " " + item.Body)
		excluded := false
		for _, term := range excludes {
			if containsCollectionQuery(text, term) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		if len(includes) > 0 && !matchesAnyCollectionQuery(includes, text) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func matchesAnyCollectionQuery(queries []string, text string) bool {
	for _, query := range queries {
		relevance, phraseMatched := collectionRelevance(query, text)
		if relevance >= 50 && (phraseMatched || relevance >= 65) {
			return true
		}
	}
	return false
}

func (service *CollectionService) fail(ctx context.Context, run domain.CollectionRun, targets []domain.PublishedCollectionTarget, result domain.FetchResult, kind domain.CollectionErrorKind, cause error) (domain.CollectionRun, error) {
	if !kind.Valid() {
		kind = domain.CollectionErrorPermanent
	}
	if service.logger != nil {
		service.logger.Warn("collection source fetch failed",
			zap.Int64("source_connection_id", run.SourceConnectionID),
			zap.String("error_kind", string(kind)),
			zap.String("reason", domain.SafeCollectionErrorCause(cause)),
		)
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

func captureMetadataPolicy() domain.CapturePolicy {
	return domain.CapturePolicy{
		Version: domain.CapturedItemVersionV2, RawPayloadDisposition: domain.RawPayloadDiscarded,
	}
}
