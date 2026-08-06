package domain

import "context"

// MonitorRepository is owned by the Monitor module. Its implementation may
// lock Monitor rows and immutable configuration rows, but the domain contract
// exposes no SQL/GORM representation.
type MonitorRepository interface {
	Create(context.Context, *Monitor, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error
	FindByID(context.Context, int64) (*Monitor, error)
	LockByID(context.Context, int64) (*Monitor, error)
	FindConfig(context.Context, int64) (*MonitorConfigVersion, []MonitorRule, []MonitorSource, error)
	LockConfig(context.Context, int64) (*MonitorConfigVersion, []MonitorRule, []MonitorSource, error)
	CreateDraft(context.Context, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error
	SaveDraft(context.Context, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error
	SaveMonitor(context.Context, *Monitor) error
	SoftDelete(context.Context, *Monitor) error
	Publish(context.Context, *Monitor, *MonitorConfigVersion, *MonitorConfigVersion, []MonitorSource) error
	List(context.Context, MonitorListQuery) ([]Monitor, string, error)
	ListActivePublished(context.Context) ([]PublishedMonitor, error)
}

// MonitorListQuery is the only Monitor list shape exposed to infrastructure.
// It fixes id-ascending cursor pagination and lets the application select the
// viewer-safe published predicate without leaking transport query syntax.
type MonitorListQuery struct {
	Cursor        string
	Limit         int
	PublishedOnly bool
}

// PublishedMonitor is the downstream-safe read model for PLAN-006. It is
// deliberately limited to an active Monitor and its immutable published
// configuration; draft facts never leave this repository method.
type PublishedMonitor struct {
	Monitor Monitor
	Config  MonitorConfigVersion
	Rules   []MonitorRule
	Sources []MonitorSource
}

// SourceConnectionSummary is deliberately a safe, source-owned read model.
// It omits endpoint, config, credential reference, health diagnostics, and
// persistence details from Monitor's cross-module dependency.
type SourceConnectionSummary struct {
	ID                   int64
	Version              int64
	Name                 string
	SourceType           string
	Enabled              bool
	Deleted              bool
	CredentialConfigured bool
}

type SourceConnectionReader interface {
	FindForMonitor(context.Context, int64) (SourceConnectionSummary, error)
}
