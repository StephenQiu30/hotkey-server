package domain

import "context"

// SourceConnectionRepository owns source_connections only. Monitor usage is
// obtained through MonitorUsageReader, never by a Source repository query.
type SourceConnectionRepository interface {
	Create(context.Context, *SourceConnection) error
	FindByID(context.Context, int64) (*SourceConnection, error)
	LockByID(context.Context, int64) (*SourceConnection, error)
	Update(context.Context, *SourceConnection) error
	HasPublishedReference(context.Context, int64) (bool, error)
}

// SourceUsage is a minimal Monitor-owned fact used by Source lifecycle
// commands. Task 4 supplies the PostgreSQL adapter in the Monitor module.
type SourceUsage struct {
	ReferencedByActiveMonitor bool
	ReferencedByPausedMonitor bool
	ActiveMonitorCount        int
	PausedMonitorCount        int
	SoleSchedulableForActive  bool
}

type MonitorUsageReader interface {
	UsageForSource(context.Context, int64) (SourceUsage, error)
}
