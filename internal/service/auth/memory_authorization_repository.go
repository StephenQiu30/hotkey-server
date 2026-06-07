package auth

import (
	"context"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/authorization"
)

type MemoryAuthorizationRepository struct {
	mu    sync.RWMutex
	store map[string]authorization.Authorization
	byUser map[string][]string // userID -> []authorizationID
}

func NewMemoryAuthorizationRepository() *MemoryAuthorizationRepository {
	return &MemoryAuthorizationRepository{
		store:  make(map[string]authorization.Authorization),
		byUser: make(map[string][]string),
	}
}

func (r *MemoryAuthorizationRepository) CreateAuthorization(_ context.Context, az authorization.Authorization) (authorization.Authorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check uniqueness
	for _, id := range r.byUser[az.UserID] {
		if existing, ok := r.store[id]; ok && existing.Platform == az.Platform {
			return authorization.Authorization{}, authorization.ErrUniqueViolation
		}
	}

	r.store[az.ID] = az
	r.byUser[az.UserID] = append(r.byUser[az.UserID], az.ID)
	return az, nil
}

func (r *MemoryAuthorizationRepository) AuthorizationByID(_ context.Context, id string) (authorization.Authorization, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	az, exists := r.store[id]
	if !exists {
		return authorization.Authorization{}, authorization.ErrNotFound
	}
	return az, nil
}

func (r *MemoryAuthorizationRepository) AuthorizationsByUserID(_ context.Context, userID string) ([]authorization.Authorization, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byUser[userID]
	result := make([]authorization.Authorization, 0, len(ids))
	for _, id := range ids {
		if az, exists := r.store[id]; exists {
			result = append(result, az)
		}
	}
	return result, nil
}

func (r *MemoryAuthorizationRepository) UpdateAuthorizationStatus(_ context.Context, id string, status authorization.Status, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	az, exists := r.store[id]
	if !exists {
		return authorization.ErrNotFound
	}
	az.Status = status
	az.UpdatedAt = now
	if status == authorization.StatusRevoked {
		az.RevokedAt = &now
	}
	r.store[id] = az
	return nil
}

func (r *MemoryAuthorizationRepository) TouchAuthorization(_ context.Context, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	az, exists := r.store[id]
	if !exists {
		return authorization.ErrNotFound
	}
	az.LastCheckedAt = now
	az.UpdatedAt = now
	r.store[id] = az
	return nil
}

func (r *MemoryAuthorizationRepository) RevokeAllByUserID(_ context.Context, userID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.byUser[userID]
	for _, id := range ids {
		if az, exists := r.store[id]; exists {
			az.Status = authorization.StatusRevoked
			az.RevokedAt = &now
			az.UpdatedAt = now
			r.store[id] = az
		}
	}
	return nil
}

func (r *MemoryAuthorizationRepository) DeleteAuthorizationsByUserID(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.byUser[userID]
	for _, id := range ids {
		delete(r.store, id)
	}
	delete(r.byUser, userID)
	return nil
}
