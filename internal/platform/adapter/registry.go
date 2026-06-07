package adapter

import (
	"crypto/sha256"
	"fmt"
	"sync"
)

// Registry manages registered platform adapters.
type Registry struct {
	adapters map[Provider]Adapter
	mu       sync.RWMutex
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[Provider]Adapter),
	}
}

// Register adds or replaces an adapter for its provider.
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Provider()] = a
}

// Get returns the adapter for the given provider, or false if not found.
func (r *Registry) Get(p Provider) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[p]
	return a, ok
}

// List returns all registered adapters.
func (r *Registry) List() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		result = append(result, a)
	}
	return result
}

// NewIdempotencyKey generates a deterministic idempotency key from sourceID and URL.
func NewIdempotencyKey(sourceID, url string) string {
	h := sha256.Sum256([]byte(sourceID + "|" + url))
	return fmt.Sprintf("%x", h[:16])
}
