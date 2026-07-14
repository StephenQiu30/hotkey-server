package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/redis/go-redis/v9"
)

func newRedisSessionRepo(t *testing.T) *repository.RedisAuthSessionRepository {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() { rdb.FlushDB(context.Background()); rdb.Close() })
	return repository.NewRedisAuthSessionRepository(rdb)
}

func TestRedisCreateSession(t *testing.T) {
	repo := newRedisSessionRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)

	session, err := repo.CreateSession(ctx, 1, "hash1", "family1", "127.0.0.1", "test-agent", expiresAt, absoluteExpiresAt)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == 0 {
		t.Fatal("expected non-zero session ID")
	}
	if session.UserID != 1 {
		t.Fatalf("expected user ID 1, got %d", session.UserID)
	}
	if session.TokenHash != "hash1" {
		t.Fatalf("expected token hash 'hash1', got %s", session.TokenHash)
	}
	if session.Status != "active" {
		t.Fatalf("expected status 'active', got %s", session.Status)
	}
	if !session.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiresAt %v, got %v", expiresAt, session.ExpiresAt)
	}
}

func TestRedisGetSession(t *testing.T) {
	repo := newRedisSessionRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	session, _ := repo.CreateSession(ctx, 1, "hash1", "family1", "127.0.0.1", "test-agent",
		now.Add(7*24*time.Hour), now.Add(30*24*time.Hour))

	// Get existing session
	got, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.UserID != 1 {
		t.Fatalf("expected user ID 1, got %d", got.UserID)
	}
	if got.TokenHash != "hash1" {
		t.Fatalf("expected token hash 'hash1', got %s", got.TokenHash)
	}

	// Get non-existent session
	missing, err := repo.GetSession(ctx, 99999)
	if err != nil {
		t.Fatalf("GetSession(non-existent): %v", err)
	}
	if missing != nil {
		t.Fatal("expected nil for non-existent session")
	}
}

func TestRedisRevokeSession(t *testing.T) {
	repo := newRedisSessionRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	session, _ := repo.CreateSession(ctx, 1, "hash1", "family1", "127.0.0.1", "test-agent",
		now.Add(7*24*time.Hour), now.Add(30*24*time.Hour))

	if err := repo.RevokeSession(ctx, session.ID, "logout"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	got, _ := repo.GetSession(ctx, session.ID)
	if got == nil {
		t.Fatal("expected session to exist after revoke")
	}
	if got.Status != "revoked" {
		t.Fatalf("expected status 'revoked', got %s", got.Status)
	}

	// Revoke non-existent session
	err := repo.RevokeSession(ctx, 99999, "logout")
	if err != repository.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestRedisRotateSession(t *testing.T) {
	repo := newRedisSessionRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	session, _ := repo.CreateSession(ctx, 1, "hash1", "family1", "127.0.0.1", "test-agent",
		now.Add(7*24*time.Hour), now.Add(30*24*time.Hour))

	// Successful rotation
	rotated, err := repo.RotateSession(ctx, session.ID, "hash1", "hash2", now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("RotateSession: %v", err)
	}
	if rotated.TokenHash != "hash2" {
		t.Fatalf("expected token hash 'hash2', got %s", rotated.TokenHash)
	}

	// Old token reuse → ErrTokenMismatch
	_, err = repo.RotateSession(ctx, session.ID, "hash1", "hash3", now.Add(2*time.Hour))
	if err != repository.ErrTokenMismatch {
		t.Fatalf("expected ErrTokenMismatch for old token reuse, got %v", err)
	}

	// Revoked session → ErrSessionRevoked
	session2, _ := repo.CreateSession(ctx, 1, "hashA", "family2", "127.0.0.1", "test-agent",
		now.Add(7*24*time.Hour), now.Add(30*24*time.Hour))
	_ = repo.RevokeSession(ctx, session2.ID, "logout")
	_, err = repo.RotateSession(ctx, session2.ID, "hashA", "hashB", now.Add(3*time.Hour))
	if err != repository.ErrSessionRevoked {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}

	// Non-existent session → ErrSessionNotFound
	_, err = repo.RotateSession(ctx, 99999, "x", "y", now)
	if err != repository.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
