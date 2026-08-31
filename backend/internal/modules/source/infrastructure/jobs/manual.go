package jobs

import (
	"context"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// ManualCollectionActivator only persists the bounded collect_source envelope.
// Connector I/O remains exclusively in CollectHandler workers.
type ManualCollectionActivator struct{ jobs *queue.Store }

func NewManualCollectionActivator(jobs *queue.Store) (*ManualCollectionActivator, error) {
	if jobs == nil {
		return nil, fmt.Errorf("manual collection activator dependencies are required")
	}
	return &ManualCollectionActivator{jobs: jobs}, nil
}

func (activator *ManualCollectionActivator) Enqueue(ctx context.Context, command sourcedomain.ManualCollectionCommand) (bool, error) {
	if activator == nil || activator.jobs == nil {
		return false, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return false, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	args := scheduler.CollectionJobArgs{
		MonitorID: command.MonitorID, MonitorVersionID: command.ConfigVersionID,
		CompiledProfileID: command.CompiledProfileID, SourceConnectionID: command.SourceConnectionID,
		WindowStart: command.WindowStart.UTC(), WindowEnd: command.WindowEnd.UTC(),
		InputHash: command.QuerySignature, TriggerType: string(sourcedomain.CollectionTriggerManual),
	}
	encoded, err := scheduler.EncodeCollectionJobArgs(args)
	if err != nil {
		return false, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	_, created, err := activator.jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindCollectSource,
		UniqueKey: scheduler.ManualCollectionUniqueKey(command.MonitorID, command.ConfigVersionID,
			command.CompiledProfileID, command.SourceConnectionID, command.ScheduledAt),
		DurableArgs: encoded,
		ScheduledAt: command.ScheduledAt.UTC(), MaxAttempts: 3, Priority: 2,
	})
	return created, err
}
