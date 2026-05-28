package repo

import "github.com/StephenQiu30/hotkey-server/internal/source"

// SourceRepo defines the storage interface for content sources.
type SourceRepo interface {
	CreateSource(src source.Source) error
	ListSources() ([]source.Source, error)
	ListSourcesByTenant(tenantID string) ([]source.Source, error)
	GetSource(tenantID, id string) (source.Source, error)
	UpdateSource(src source.Source) error
}
