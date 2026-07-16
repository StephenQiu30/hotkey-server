package postgres

import (
	"context"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// SourceUsageReader is a Monitor-owned, read-only adapter supplied to Source
// lifecycle commands. Source never imports Monitor SQL or tables. The caller
// already owns the global configuration advisory lock; this adapter locks the
// affected monitor/config/source relation rows in that same transaction before
// calculating the sole-schedulable predicate.
type SourceUsageReader struct{ runtime *database.Runtime }

var _ sourcedomain.MonitorUsageReader = (*SourceUsageReader)(nil)

func NewSourceUsageReader(runtime *database.Runtime) *SourceUsageReader {
	return &SourceUsageReader{runtime: runtime}
}

func (reader *SourceUsageReader) UsageForSource(ctx context.Context, sourceID int64) (sourcedomain.SourceUsage, error) {
	if sourceID <= 0 {
		return sourcedomain.SourceUsage{}, fmt.Errorf("%w: source id is required", sharedrepository.ErrInvalidInput)
	}
	if reader == nil || reader.runtime == nil {
		return sourcedomain.SourceUsage{}, sharedrepository.ErrUnavailable
	}
	transaction, inTransaction := database.TransactionFromContext(ctx)
	if !inTransaction {
		return sourcedomain.SourceUsage{}, fmt.Errorf("%w: monitor source usage requires caller transaction", sharedrepository.ErrUnavailable)
	}
	// Lock every published relation that references this SourceConnection. The
	// explicit rows prevent a concurrent publish/resume from changing the set
	// after Source has acquired the configuration lock.
	locked, err := transaction.SQL.QueryContext(ctx, `
SELECT monitor_source.id
FROM monitor_sources AS monitor_source
JOIN monitor_config_versions AS config_version ON config_version.id = monitor_source.config_version_id
JOIN monitors AS monitor ON monitor.published_config_version_id = config_version.id
WHERE monitor_source.source_connection_id = $1
  AND monitor.status IN ('active', 'paused')
FOR UPDATE OF monitor_source, config_version, monitor`, sourceID)
	if err != nil {
		return sourcedomain.SourceUsage{}, sharedrepository.MapError(err)
	}
	if err := locked.Close(); err != nil {
		return sourcedomain.SourceUsage{}, sharedrepository.MapError(err)
	}

	var usage sourcedomain.SourceUsage
	err = transaction.SQL.QueryRowContext(ctx, `
WITH referenced AS (
    SELECT monitor.id AS monitor_id, monitor.status
    FROM monitors AS monitor
    JOIN monitor_config_versions AS config_version ON config_version.id = monitor.published_config_version_id
    JOIN monitor_sources AS monitor_source ON monitor_source.config_version_id = config_version.id
    WHERE monitor_source.source_connection_id = $1
      AND monitor_source.enabled
      AND monitor.status IN ('active', 'paused')
), schedulable AS (
    SELECT monitor.id AS monitor_id, count(*)::integer AS source_count
    FROM monitors AS monitor
    JOIN monitor_config_versions AS config_version ON config_version.id = monitor.published_config_version_id
    JOIN monitor_sources AS monitor_source ON monitor_source.config_version_id = config_version.id
    JOIN source_connections AS connection ON connection.id = monitor_source.source_connection_id
    WHERE monitor.status = 'active'
      AND monitor_source.enabled
      AND connection.enabled
      AND connection.deleted_at IS NULL
    GROUP BY monitor.id
)
SELECT
    COALESCE(bool_or(referenced.status = 'active'), false),
    COALESCE(bool_or(referenced.status = 'paused'), false),
    count(*) FILTER (WHERE referenced.status = 'active')::integer,
    count(*) FILTER (WHERE referenced.status = 'paused')::integer,
    COALESCE(bool_or(referenced.status = 'active' AND schedulable.source_count = 1), false)
FROM referenced
LEFT JOIN schedulable ON schedulable.monitor_id = referenced.monitor_id`, sourceID).Scan(&usage.ReferencedByActiveMonitor, &usage.ReferencedByPausedMonitor, &usage.ActiveMonitorCount, &usage.PausedMonitorCount, &usage.SoleSchedulableForActive)
	if err != nil {
		return sourcedomain.SourceUsage{}, sharedrepository.MapError(err)
	}
	return usage, nil
}
