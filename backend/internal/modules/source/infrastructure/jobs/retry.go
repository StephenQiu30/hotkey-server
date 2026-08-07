package jobs

import (
	"context"
	"errors"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
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

func (activator *CollectionRetryActivator) Reactivate(ctx context.Context, retry sourcedomain.CollectionRunRetry) error {
	if activator == nil || activator.targets == nil || activator.jobs == nil {
		return sharedrepository.ErrUnavailable
	}
	if len(retry.Targets) == 0 {
		return fmt.Errorf("%w: original collection targets are unavailable", sharedrepository.ErrConflict)
	}
	minimumConfigVersionID := retry.Targets[0].MonitorConfigVersionID
	expected := make(map[sourcedomain.CollectionRunTargetIdentity]struct{}, len(retry.Targets))
	for _, target := range retry.Targets {
		if target.MonitorSourceID <= 0 || target.MonitorConfigVersionID <= 0 {
			return fmt.Errorf("%w: original collection target identity is invalid", sharedrepository.ErrConflict)
		}
		if target.MonitorConfigVersionID < minimumConfigVersionID {
			minimumConfigVersionID = target.MonitorConfigVersionID
		}
		if _, duplicate := expected[target]; duplicate {
			return fmt.Errorf("%w: original collection target identity is duplicated", sharedrepository.ErrConflict)
		}
		expected[target] = struct{}{}
	}
	run := retry.Run
	triggerType := run.TriggerType
	if triggerType == sourcedomain.CollectionTriggerRetry || triggerType == sourcedomain.CollectionTriggerReconcile || triggerType == "" {
		triggerType = sourcedomain.CollectionTriggerSchedule
	}
	targets, err := activator.targets.ListForCollection(ctx, run.SourceConnectionID, minimumConfigVersionID, run.QuerySignature, run.WindowStart, run.WindowEnd, triggerType)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return fmt.Errorf("%w: collection targets are no longer eligible", sharedrepository.ErrConflict)
		}
		return err
	}
	if len(targets) != len(expected) {
		return fmt.Errorf("%w: collection target set is no longer eligible", sharedrepository.ErrConflict)
	}
	for _, target := range targets {
		identity := sourcedomain.CollectionRunTargetIdentity{
			MonitorSourceID: target.MonitorSourceID, MonitorConfigVersionID: target.MonitorConfigVersionID,
		}
		if _, found := expected[identity]; !found {
			return fmt.Errorf("%w: collection target set changed", sharedrepository.ErrConflict)
		}
	}
	uniqueKey := scheduler.CollectionUniqueKey(run.SourceConnectionID, run.QuerySignature, run.WindowStart, run.WindowEnd)
	if triggerType == sourcedomain.CollectionTriggerManual {
		uniqueKey = scheduler.ManualCollectionUniqueKey(run.SourceConnectionID, run.QuerySignature, run.ScheduledAt)
	}
	_, err = activator.jobs.ReactivateByUniqueKey(ctx, queue.KindCollectSource, uniqueKey)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return fmt.Errorf("%w: original collection job is unavailable", sharedrepository.ErrConflict)
		}
		return err
	}
	return nil
}
