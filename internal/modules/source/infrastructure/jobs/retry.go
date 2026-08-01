package jobs

import (
	"context"
	"errors"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/internal/platform/scheduler"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// CollectionRetryActivator validates the currently published target through
// the caller transaction before reactivating the original durable job.
type CollectionRetryActivator struct {
	targets CollectionTargetReader
	jobs    *queue.Store
}

func NewCollectionRetryActivator(targets CollectionTargetReader, jobs *queue.Store) (*CollectionRetryActivator, error) {
	if targets == nil || jobs == nil {
		return nil, fmt.Errorf("collection retry activator dependencies are required")
	}
	return &CollectionRetryActivator{targets: targets, jobs: jobs}, nil
}

func (activator *CollectionRetryActivator) Reactivate(ctx context.Context, run sourcedomain.CollectionRun) error {
	if activator == nil || activator.targets == nil || activator.jobs == nil {
		return sharedrepository.ErrUnavailable
	}
	targets, err := activator.targets.ListForCollection(ctx, run.SourceConnectionID, 1, run.QuerySignature, run.WindowStart, run.WindowEnd)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return fmt.Errorf("%w: collection targets are no longer eligible", sharedrepository.ErrConflict)
		}
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("%w: collection targets are no longer eligible", sharedrepository.ErrConflict)
	}
	_, err = activator.jobs.ReactivateByUniqueKey(ctx, queue.KindCollectSource, scheduler.CollectionUniqueKey(run.SourceConnectionID, run.QuerySignature, run.WindowStart, run.WindowEnd))
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return fmt.Errorf("%w: original collection job is unavailable", sharedrepository.ErrConflict)
		}
		return err
	}
	return nil
}
