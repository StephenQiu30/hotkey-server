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

// PublicSourceConnection is the only Source fact available to ordinary
// readers. Credential references, endpoints, configuration and health
// diagnostics never leave Source management code through this model.
type PublicSourceConnection struct {
	ID                   int64
	Version              int64
	Name                 string
	SourceType           SourceType
	Enabled              bool
	HealthStatus         HealthStatus
	TermsPolicyURL       string
	CredentialConfigured bool
	Deleted              bool
}

// ManagementSourceConnection is an administrator-only projection. Config is
// the validated, non-secret P0 allowlist; CredentialRef is intentionally not
// present even for management reads.
type ManagementSourceConnection struct {
	PublicSourceConnection
	Endpoint string
	Config   SourceConfig
}

// MonitorSourceConnection is the source-owned input a Monitor use case may
// lock and inspect. It is not a persistence record and it never carries a
// credential reference. The source config is safe only because the Source
// domain has already applied the fixed P0 allowlist.
type MonitorSourceConnection struct {
	ID         int64
	Version    int64
	Name       string
	SourceType SourceType
	Endpoint   string
	Config     SourceConfig
	Enabled    bool
	Deleted    bool
}

// MonitorSourceReader is implemented by the Source application service. It
// lets the Monitor application obtain Source-owned, credential-free facts
// without importing a PostgreSQL record or reaching into source_connections.
type MonitorSourceReader interface {
	FindForMonitor(context.Context, int64) (MonitorSourceConnection, error)
	LockForMonitor(context.Context, int64) (MonitorSourceConnection, error)
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
