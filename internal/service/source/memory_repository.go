package source

import (
	"context"
	"sync"
)

type MemoryRepository struct {
	mu          sync.RWMutex
	sources     map[string]Source
	sourceOrder []string
	runs        map[string][]CollectionRun
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		sources: make(map[string]Source),
		runs:    make(map[string][]CollectionRun),
	}
}

func (r *MemoryRepository) ListSources(_ context.Context) ([]Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listSourcesLocked(false), nil
}

func (r *MemoryRepository) ListCollectableSources(_ context.Context) ([]Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listSourcesLocked(true), nil
}

func (r *MemoryRepository) SourceByID(_ context.Context, sourceID string) (Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, exists := r.sources[sourceID]
	if !exists {
		return Source{}, ErrNotFound
	}
	return cloneSource(source), nil
}

func (r *MemoryRepository) CreateSource(_ context.Context, source Source) (Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[source.ID]; exists {
		return Source{}, ErrAlreadyExists
	}
	r.sources[source.ID] = cloneSource(source)
	r.sourceOrder = append(r.sourceOrder, source.ID)
	return cloneSource(source), nil
}

func (r *MemoryRepository) UpdateSource(_ context.Context, source Source) (Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[source.ID]; !exists {
		return Source{}, ErrNotFound
	}
	r.sources[source.ID] = cloneSource(source)
	return cloneSource(source), nil
}

func (r *MemoryRepository) CreateCollectionRun(_ context.Context, run CollectionRun) (CollectionRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[run.SourceID]; !exists {
		return CollectionRun{}, ErrNotFound
	}
	r.runs[run.SourceID] = append(r.runs[run.SourceID], run)
	source := r.sources[run.SourceID]
	source.LastCollectedAt = &run.FinishedAt
	source.LastError = run.Error
	r.sources[run.SourceID] = source
	return run, nil
}

func (r *MemoryRepository) ListCollectionRuns(_ context.Context, sourceID string) ([]CollectionRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	runs := append([]CollectionRun(nil), r.runs[sourceID]...)
	return runs, nil
}

func (r *MemoryRepository) listSourcesLocked(collectableOnly bool) []Source {
	sources := make([]Source, 0, len(r.sourceOrder))
	for _, id := range r.sourceOrder {
		source, exists := r.sources[id]
		if !exists {
			continue
		}
		if collectableOnly && source.Status != SourceStatusEnabled {
			continue
		}
		sources = append(sources, cloneSource(source))
	}
	return sources
}

func cloneSource(source Source) Source {
	source.ChannelIDs = append([]string(nil), source.ChannelIDs...)
	return source
}
