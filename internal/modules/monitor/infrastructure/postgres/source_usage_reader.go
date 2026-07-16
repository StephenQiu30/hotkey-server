package postgres

import (
	"context"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// SourceUsageReader is a Monitor-owned, read-only adapter supplied to Source
// lifecycle commands. It queries and locks only Monitor-owned tables; Source
// application evaluates SourceConnection availability using its own repository
// while retaining the same transaction and global configuration lock.
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

	// `target` narrows the affected published Monitor configurations. `member`
	// then returns their Monitor-owned association facts, not SourceConnection
	// state. Locking all three tables stabilizes those groups after the caller
	// has acquired hotkey.monitor_source_configuration.
	rows, err := transaction.SQL.QueryContext(ctx, `
SELECT monitor.id, monitor.status, member.source_connection_id, member.enabled
FROM monitors AS monitor
JOIN monitor_config_versions AS config_version ON config_version.id = monitor.published_config_version_id
JOIN monitor_sources AS target ON target.config_version_id = config_version.id
JOIN monitor_sources AS member ON member.config_version_id = config_version.id
WHERE target.source_connection_id = $1
  AND target.enabled
  AND monitor.status IN ('active', 'paused')
ORDER BY monitor.id ASC, member.id ASC
FOR UPDATE OF monitor, config_version, target, member`, sourceID)
	if err != nil {
		return sourcedomain.SourceUsage{}, sharedrepository.MapError(err)
	}
	defer rows.Close()

	type grouped struct {
		status string
		group  sourcedomain.MonitorUsageGroup
	}
	groups := map[int64]*grouped{}
	order := []int64{}
	for rows.Next() {
		var monitorID, memberID int64
		var status string
		var enabled bool
		if err := rows.Scan(&monitorID, &status, &memberID, &enabled); err != nil {
			return sourcedomain.SourceUsage{}, sharedrepository.MapError(err)
		}
		item, found := groups[monitorID]
		if !found {
			item = &grouped{status: status, group: sourcedomain.MonitorUsageGroup{MonitorID: monitorID, Sources: []sourcedomain.MonitorUsageSource{}}}
			groups[monitorID] = item
			order = append(order, monitorID)
		}
		item.group.Sources = append(item.group.Sources, sourcedomain.MonitorUsageSource{SourceConnectionID: memberID, Enabled: enabled})
	}
	if err := rows.Err(); err != nil {
		return sourcedomain.SourceUsage{}, sharedrepository.MapError(err)
	}

	usage := sourcedomain.SourceUsage{ActiveMonitorGroups: []sourcedomain.MonitorUsageGroup{}, PausedMonitorGroups: []sourcedomain.MonitorUsageGroup{}}
	for _, monitorID := range order {
		item := groups[monitorID]
		switch item.status {
		case "active":
			usage.ActiveMonitorGroups = append(usage.ActiveMonitorGroups, item.group)
		case "paused":
			usage.PausedMonitorGroups = append(usage.PausedMonitorGroups, item.group)
		default:
			return sourcedomain.SourceUsage{}, fmt.Errorf("%w: unexpected monitor status", sharedrepository.ErrConstraint)
		}
	}
	return usage, nil
}
