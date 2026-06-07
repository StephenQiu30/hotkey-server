package eventsummary

import (
	"context"
	"sync"
)

// MemoryRepository is an in-memory implementation of SummaryRepository.
type MemoryRepository struct {
	mu       sync.RWMutex
	byEventID map[string]EventSummary
}

// NewMemoryRepository creates a new empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byEventID: make(map[string]EventSummary)}
}

// Save stores an EventSummary, keyed by EventID.
func (r *MemoryRepository) Save(_ context.Context, s EventSummary) (EventSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byEventID[s.EventID] = s
	return s, nil
}

// FindByEventID returns the EventSummary for the given event ID.
func (r *MemoryRepository) FindByEventID(_ context.Context, eventID string) (EventSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byEventID[eventID]
	if !ok {
		return EventSummary{}, ErrNotFound
	}
	return s, nil
}
