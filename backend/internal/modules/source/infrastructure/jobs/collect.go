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
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
)

type CollectionTargetReader interface {
	ListForCollection(context.Context, int64, int64, string, time.Time, time.Time, sourcedomain.CollectionTriggerType) ([]sourcedomain.PublishedCollectionTarget, error)
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
	args, err := scheduler.DecodeCollectionJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	triggerType := sourcedomain.CollectionTriggerType(args.TriggerType)
	resolve := func(transactionCtx context.Context) (sourcedomain.CollectionRequest, error) {
		targets, err := handler.targets.ListForCollection(transactionCtx, args.SourceConnectionID, args.MonitorVersionID, args.InputHash, args.WindowStart, args.WindowEnd, triggerType)
		if err != nil {
			return sourcedomain.CollectionRequest{}, err
		}
		exactPublication := false
		for _, target := range targets {
			if target.MonitorID == args.MonitorID && target.MonitorConfigVersionID == args.MonitorVersionID &&
				target.CompiledProfileID == args.CompiledProfileID {
				exactPublication = true
				break
			}
		}
		if !exactPublication {
			return sourcedomain.CollectionRequest{}, fmt.Errorf("collection publication identity is no longer eligible")
		}
		planner := sourceapplication.QueryPlanner{}
		requests := make([]sourcedomain.CollectionRequest, 0, len(targets))
		for _, target := range targets {
			request, err := planner.Plan(target, args.WindowStart, args.WindowEnd)
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
		groups[0].ScheduledAt = job.ScheduledAt.UTC()
		groups[0].TriggerType = triggerType
		return groups[0], nil
	}
	run, err := handler.collections.CollectResolvedWithSuccessHook(ctx, args.SourceConnectionID, args.InputHash, resolve, func(transactionCtx context.Context, runID int64) error {
		_, _, err := handler.jobs.Enqueue(transactionCtx, queue.Job{
			Kind:        queue.KindNormalizeContent,
			UniqueKey:   queue.StableJobKey(queue.KindNormalizeContent, runID, 1, args.InputHash),
			Payload:     queue.Payload{EntityID: runID, EntityVersion: 1, WindowStart: args.WindowStart, WindowEnd: args.WindowEnd, InputHash: args.InputHash},
			ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 2,
		})
		return err
	})
	if err != nil {
		if sourcedomain.IsCollectionRetryable(err) {
			if run.RetryAfter != nil {
				return queue.NewRetryableErrorAt(err, *run.RetryAfter)
			}
			return queue.NewRetryableError(err)
		}
		return queue.NewPermanentError(err)
	}
	return nil
}
