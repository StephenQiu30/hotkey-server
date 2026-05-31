package auth

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
)

type MemoryRepository struct {
	mu            sync.RWMutex
	usersByID     map[string]user.User
	userIDByEmail map[string]string
	refreshByHash map[string]RefreshToken
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:     make(map[string]user.User),
		userIDByEmail: make(map[string]string),
		refreshByHash: make(map[string]RefreshToken),
	}
}

func (r *MemoryRepository) CreateUser(_ context.Context, account user.User) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.userIDByEmail[account.Email]; exists {
		return user.User{}, ErrEmailAlreadyExists
	}
	r.usersByID[account.ID] = account
	r.userIDByEmail[account.Email] = account.ID
	return account, nil
}

func (r *MemoryRepository) UserByEmail(_ context.Context, email string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, exists := r.userIDByEmail[email]
	if !exists {
		return user.User{}, sql.ErrNoRows
	}
	return r.usersByID[id], nil
}

func (r *MemoryRepository) UserByID(_ context.Context, id string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	account, exists := r.usersByID[id]
	if !exists {
		return user.User{}, sql.ErrNoRows
	}
	return account, nil
}

func (r *MemoryRepository) CreateRefreshToken(_ context.Context, token RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshByHash[token.TokenHash] = token
	return nil
}

func (r *MemoryRepository) RefreshTokenByHash(_ context.Context, tokenHash string) (RefreshToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	token, exists := r.refreshByHash[tokenHash]
	if !exists {
		return RefreshToken{}, sql.ErrNoRows
	}
	return token, nil
}

func (r *MemoryRepository) RevokeRefreshToken(_ context.Context, tokenHash string, revokedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, exists := r.refreshByHash[tokenHash]
	if !exists {
		return sql.ErrNoRows
	}
	token.RevokedAt = &revokedAt
	r.refreshByHash[tokenHash] = token
	return nil
}
