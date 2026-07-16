package domain

import "context"

// MonitorRepository is owned by the Monitor module. Its implementation may
// lock Monitor rows and immutable configuration rows, but the domain contract
// exposes no SQL/GORM representation.
type MonitorRepository interface {
	Create(context.Context, *Monitor, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error
	FindByID(context.Context, int64) (*Monitor, error)
	LockByID(context.Context, int64) (*Monitor, error)
	FindConfigVersion(context.Context, int64) (*MonitorConfigVersion, error)
	SaveDraft(context.Context, *Monitor, *MonitorConfigVersion, []MonitorRule, []MonitorSource) error
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
