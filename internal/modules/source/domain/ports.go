package domain

import (
	"context"
	"time"
)

// SourceConnectionRepository owns source_connections only. Monitor usage is
// obtained through MonitorUsageReader, never by a Source repository query.
type SourceConnectionRepository interface {
	Create(context.Context, *SourceConnection) error
	FindByID(context.Context, int64) (*SourceConnection, error)
	LockByID(context.Context, int64) (*SourceConnection, error)
	List(context.Context, SourceConnectionListQuery) ([]SourceConnection, string, error)
	Update(context.Context, *SourceConnection) error
}

// SourceConnectionListQuery is intentionally narrow: Source lists are always
// ordered by ID ascending and use a fixed filter shape. This prevents a future
// HTTP transport from passing arbitrary sort/filter SQL into the repository.
type SourceConnectionListQuery struct {
	Cursor string
	Limit  int
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

type PublicSourceConnectionPage struct {
	Items      []PublicSourceConnection
	NextCursor string
}

type ManagementSourceConnectionPage struct {
	Items      []ManagementSourceConnection
	NextCursor string
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

// MonitorUsageGroup is a Monitor-owned published configuration relation. It
// intentionally identifies only the associated SourceConnection IDs and the
// Monitor-side association state. Source application remains responsible for
// looking up whether those SourceConnections are enabled or archived.
type MonitorUsageGroup struct {
	MonitorID int64
	Sources   []MonitorUsageSource
}

type MonitorUsageSource struct {
	SourceConnectionID int64
	Enabled            bool
}

// SourceUsage is a narrow Monitor-owned fact used by Source lifecycle
// commands. It contains no SourceConnection availability result and lets the
// Source application evaluate that predicate with its own repository.
type SourceUsage struct {
	ActiveMonitorGroups []MonitorUsageGroup
	PausedMonitorGroups []MonitorUsageGroup
}

type MonitorUsageReader interface {
	UsageForSource(context.Context, int64) (SourceUsage, error)
}

// MonitorPublishedReferenceReader is the narrow Monitor-owned history query
// used before changing SourceConnection execution semantics. Its production
// adapter requires the Source application transaction and configuration lock,
// so the check is serialized with publish while remaining outside the Source
// repository's table ownership.
type MonitorPublishedReferenceReader interface {
	HasPublishedReference(context.Context, int64) (bool, error)
}

// PublishedCollectionTargetReader is Source's own narrow read port for
// immutable collection inputs. Its Monitor-owned adapter returns only the
// values required to plan a shared request; it never exposes a Monitor record
// or a draft configuration.
type PublishedCollectionTargetReader interface {
	ListDue(context.Context, time.Time) ([]PublishedCollectionTarget, error)
}

// CollectionRepository owns durable collection runs, targets, captured items
// and checkpoints. Task 2 defines only the create-or-reuse identity boundary;
// Task 6 adds its transactional write operations beside the implementation.
type CollectionRepository interface {
	CreateOrReuseRun(context.Context, CollectionRequest) (CollectionRun, bool, error)
}
