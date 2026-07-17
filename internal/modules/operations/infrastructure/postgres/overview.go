package postgres

import (
	"context"
	"database/sql"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var _ operationsapplication.OverviewStore = (*JobRepository)(nil)

func (repository *JobRepository) RuntimeOverview(ctx context.Context) (operationsdomain.RuntimeOverview, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return operationsdomain.RuntimeOverview{}, sharedrepository.ErrUnavailable
	}
	var overview operationsdomain.RuntimeOverview
	var oldest sql.NullTime
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE state = 'available'),
       count(*) FILTER (WHERE state = 'running'),
       count(*) FILTER (WHERE state = 'completed'),
       count(*) FILTER (WHERE state = 'discarded'),
       count(*) FILTER (WHERE state = 'cancelled'),
       min(scheduled_at) FILTER (WHERE state = 'available')
FROM river_job`).Scan(&overview.AvailableJobs, &overview.RunningJobs, &overview.CompletedJobs, &overview.DiscardedJobs, &overview.CancelledJobs, &oldest)
	if err != nil {
		return operationsdomain.RuntimeOverview{}, sharedrepository.MapError(err)
	}
	if oldest.Valid {
		value := oldest.Time.UTC()
		overview.OldestAvailableAt = &value
	}
	if err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT now()`).Scan(&overview.GeneratedAt); err != nil {
		return operationsdomain.RuntimeOverview{}, sharedrepository.MapError(err)
	}
	return overview, nil
}

var _ interface {
	RuntimeOverview(context.Context) (operationsdomain.RuntimeOverview, error)
} = (*JobRepository)(nil)
