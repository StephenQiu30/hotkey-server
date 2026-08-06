// Package jobs contains Source-owned durable queue handlers. Handlers receive
// only queue envelopes, then reread published facts through application ports.
package jobs

import (
	"context"
	"fmt"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type CollectionTargetReader interface {
	ListForCollection(context.Context, int64, int64, string, time.Time, time.Time) ([]sourcedomain.PublishedCollectionTarget, error)
}

// CollectHandler executes one shared source/signature/window request. The
// query and target list are deliberately reconstructed from the published
// Monitor projection rather than persisted in River args.
type CollectHandler struct {
	collections *sourceapplication.CollectionService
	targets     CollectionTargetReader
	jobs        *queue.Store
}

func NewCollectHandler(collections *sourceapplication.CollectionService, targets CollectionTargetReader, jobs *queue.Store) (*CollectHandler, error) {
	if collections == nil || targets == nil || jobs == nil {
		return nil, fmt.Errorf("collect handler dependencies are required")
	}
	return &CollectHandler{collections: collections, targets: targets, jobs: jobs}, nil
}

func (handler *CollectHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindCollectSource); err != nil {
		return queue.NewPermanentError(err)
	}
	payload := job.Payload
	resolve := func(transactionCtx context.Context) (sourcedomain.CollectionRequest, error) {
		targets, err := handler.targets.ListForCollection(transactionCtx, payload.EntityID, payload.EntityVersion, payload.InputHash, payload.WindowStart, payload.WindowEnd)
		if err != nil {
			return sourcedomain.CollectionRequest{}, err
		}
		planner := sourceapplication.QueryPlanner{}
		requests := make([]sourcedomain.CollectionRequest, 0, len(targets))
		for _, target := range targets {
			request, err := planner.Plan(target, payload.WindowStart, payload.WindowEnd)
			if err != nil {
				return sourcedomain.CollectionRequest{}, err
			}
			requests = append(requests, request)
		}
		groups, err := planner.GroupRequests(requests)
		if err != nil {
			return sourcedomain.CollectionRequest{}, err
		}
		if len(groups) != 1 {
			return sourcedomain.CollectionRequest{}, fmt.Errorf("collect envelope resolved to %d request groups", len(groups))
		}
		return groups[0], nil
	}
	_, err := handler.collections.CollectResolvedWithSuccessHook(ctx, payload.EntityID, payload.InputHash, resolve, func(transactionCtx context.Context, runID int64) error {
		_, _, err := handler.jobs.Enqueue(transactionCtx, queue.Job{
			Kind:        queue.KindNormalizeContent,
			UniqueKey:   queue.StableJobKey(queue.KindNormalizeContent, runID, 1, payload.InputHash),
			Payload:     queue.Payload{EntityID: runID, EntityVersion: 1, WindowStart: payload.WindowStart, WindowEnd: payload.WindowEnd, InputHash: payload.InputHash},
			ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 2,
		})
		return err
	})
	if err != nil {
		if sourcedomain.IsCollectionRetryable(err) {
			return queue.NewRetryableError(err)
		}
		return queue.NewPermanentError(err)
	}
	return nil
}
