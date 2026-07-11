package fakeauthsession

import (
	"context"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
)

// Repo is an in-memory fake implementing service.AuthSessionRepository.
type Repo struct {
	mu       sync.Mutex
	sessions map[int64]*entity.AuthSession
	nextID   int64
}

// NewRepo creates an empty fake repo.
func NewRepo() *Repo {
	return &Repo{
		sessions: make(map[int64]*entity.AuthSession),
		nextID:   1,
	}
}

// CreateSession inserts a new session and returns it.
func (r *Repo) CreateSession(
	ctx context.Context,
	userID int64,
	tokenHash, familyHash, ip, ua string,
	expiresAt, absoluteExpiresAt time.Time,
) (entity.AuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	id := r.nextID
	r.nextID++
	session := entity.AuthSession{
		ID:                id,
		UserID:            userID,
		TokenHash:         tokenHash,
		FamilyHash:        familyHash,
		Status:            "active",
		IPAddress:         ip,
		UserAgent:         ua,
		ExpiresAt:         expiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		LastRefreshedAt:   &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r.sessions[id] = &session
	return session, nil
}

// GetSession retrieves a session by its primary key.
func (r *Repo) GetSession(ctx context.Context, sessionID int64) (*entity.AuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	return session, nil
}

// RevokeSession marks a single session as revoked.
func (r *Repo) RevokeSession(ctx context.Context, sessionID int64, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[sessionID]
	if !ok {
		return repository.ErrSessionNotFound
	}
	session.Status = "revoked"
	session.UpdatedAt = time.Now()
	return nil
}

// RevokeUserSessions marks all active sessions for a user as revoked.
func (r *Repo) RevokeUserSessions(ctx context.Context, userID int64, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, session := range r.sessions {
		if session.UserID == userID && session.Status == "active" {
			session.Status = "revoked"
			session.UpdatedAt = time.Now()
		}
	}
	return nil
}

// RotateSession atomically rotates the token hash for a session.
// Simulates the real repo's behavior including time-bound checks,
// hash mismatch detection, and family revoke.
func (r *Repo) RotateSession(
	ctx context.Context,
	sessionID int64,
	currentTokenHash, newTokenHash string,
	now time.Time,
) (entity.AuthSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[sessionID]
	if !ok {
		return entity.AuthSession{}, repository.ErrSessionNotFound
	}

	// Reject revoked sessions.
	if session.Status == "revoked" {
		return entity.AuthSession{}, repository.ErrSessionRevoked
	}

	// Reject expired sessions.
	if now.After(session.AbsoluteExpiresAt) || now.After(session.ExpiresAt) {
		return entity.AuthSession{}, repository.ErrSessionExpired
	}

	// If the token hash doesn't match, this could be a token reuse attack.
	// Revoke ALL sessions in the same family to invalidate any stolen tokens.
	if session.TokenHash != currentTokenHash {
		for _, s := range r.sessions {
			if s.FamilyHash == session.FamilyHash && s.Status == "active" {
				s.Status = "revoked"
				s.UpdatedAt = now
			}
		}
		return entity.AuthSession{}, repository.ErrTokenMismatch
	}

	// Extend idle expiry by 7 days (capped by absolute expiry).
	newExpiresAt := now.Add(7 * 24 * time.Hour)
	if newExpiresAt.After(session.AbsoluteExpiresAt) {
		newExpiresAt = session.AbsoluteExpiresAt
	}

	session.TokenHash = newTokenHash
	session.ExpiresAt = newExpiresAt
	session.LastRefreshedAt = &now
	session.UpdatedAt = now

	// Return a copy to simulate DB snapshot semantics.
	copy := *session
	return copy, nil
}
