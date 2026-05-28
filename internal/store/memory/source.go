package memory

import (
	"sort"
	"strings"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/source"
)

// SourceRepo is an in-memory implementation of repo.SourceRepo.
type SourceRepo struct {
	mu      sync.Mutex
	sources map[string]source.Source
}

func NewSourceRepo() *SourceRepo {
	return &SourceRepo{
		sources: make(map[string]source.Source),
	}
}

func sourceKey(tenantID, id string) string {
	return strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(id)
}

func (r *SourceRepo) CreateSource(src source.Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[sourceKey(src.TenantID, src.ID)] = src
	return nil
}

func (r *SourceRepo) ListSources() ([]source.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sources := make([]source.Source, 0, len(r.sources))
	for _, src := range r.sources {
		categories := append([]string(nil), src.Categories...)
		src.Categories = categories
		sources = append(sources, src)
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID < sources[j].ID
	})
	return sources, nil
}

func (r *SourceRepo) ListSourcesByTenant(tenantID string) ([]source.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sources := make([]source.Source, 0)
	for _, src := range r.sources {
		if src.TenantID == strings.TrimSpace(tenantID) {
			categories := append([]string(nil), src.Categories...)
			src.Categories = categories
			sources = append(sources, src)
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID < sources[j].ID
	})
	return sources, nil
}

func (r *SourceRepo) GetSource(tenantID, id string) (source.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	src, ok := r.sources[sourceKey(tenantID, id)]
	if !ok {
		return source.Source{}, source.ErrSourceNotFound
	}
	categories := append([]string(nil), src.Categories...)
	src.Categories = categories
	return src, nil
}

func (r *SourceRepo) UpdateSource(src source.Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[sourceKey(src.TenantID, src.ID)]; !ok {
		return source.ErrSourceNotFound
	}
	r.sources[sourceKey(src.TenantID, src.ID)] = src
	return nil
}
