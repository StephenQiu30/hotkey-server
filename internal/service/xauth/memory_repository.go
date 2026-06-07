package xauth

import (
	"context"
	"sync"
	"time"
)

// pendingStateTTL is the maximum age of a pending OAuth state before it is considered expired.
const pendingStateTTL = 10 * time.Minute

// MemoryRepository is a thread-safe in-memory implementation of Repository.
type MemoryRepository struct {
	mu       sync.RWMutex
	states   map[string]PendingState
	creds    map[string]Credential
	now      func() time.Time
}

// NewMemoryRepository creates a new MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		states: make(map[string]PendingState),
		creds:  make(map[string]Credential),
		now:    time.Now,
	}
}

func (r *MemoryRepository) StorePendingState(_ context.Context, state PendingState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[state.State] = state
	return nil
}

func (r *MemoryRepository) GetPendingState(_ context.Context, state string) (PendingState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pending, exists := r.states[state]
	if !exists {
		return PendingState{}, ErrNotFound
	}
	// Expire stale pending states.
	if r.now().Sub(pending.CreatedAt) > pendingStateTTL {
		delete(r.states, state)
		return PendingState{}, ErrNotFound
	}
	return pending, nil
}

func (r *MemoryRepository) DeletePendingState(_ context.Context, state string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.states, state)
	return nil
}

func (r *MemoryRepository) StoreCredential(_ context.Context, cred Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creds[cred.SourceID] = cred
	return nil
}

func (r *MemoryRepository) GetCredential(_ context.Context, sourceID string) (Credential, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cred, exists := r.creds[sourceID]
	if !exists {
		return Credential{}, ErrNotFound
	}
	return cred, nil
}

func (r *MemoryRepository) DeleteCredential(_ context.Context, sourceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.creds[sourceID]; !exists {
		return ErrNotFound
	}
	delete(r.creds, sourceID)
	return nil
}
