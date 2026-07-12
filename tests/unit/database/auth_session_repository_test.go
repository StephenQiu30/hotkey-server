package database_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

func seedSession(t *testing.T, repo *repository.AuthSessionRepo, ctx context.Context, expiresAt, absoluteExpiresAt time.Time) repository.AuthSession {
	t.Helper()
	session, err := repo.CreateSession(ctx, 1, "token-hash-1", "family-1", "127.0.0.1", "test-agent", expiresAt, absoluteExpiresAt)
	if err != nil {
		t.Fatalf("seedSession: %v", err)
	}
	return session
}

func TestCreateSession(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	absoluteExpiresAt := time.Now().Add(30 * 24 * time.Hour)

	session, err := repo.CreateSession(ctx, 1, "token-hash-1", "family-1", "127.0.0.1", "test-agent", expiresAt, absoluteExpiresAt)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == 0 {
		t.Fatal("expected non-zero session ID")
	}
	if session.Status != "active" {
		t.Fatalf("expected status active, got %s", session.Status)
	}
	if session.TokenHash != "token-hash-1" {
		t.Fatalf("expected token-hash-1, got %s", session.TokenHash)
	}
}

func TestGetSession(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	session := seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	got, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.TokenHash != "token-hash-1" {
		t.Fatalf("expected token-hash-1, got %s", got.TokenHash)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	got, err := repo.GetSession(ctx, 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil session for non-existent ID")
	}
}

func TestRotateSession(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	session := seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	rotated, err := repo.RotateSession(ctx, session.ID, session.TokenHash, "new-token-hash", now)
	if err != nil {
		t.Fatalf("RotateSession: %v", err)
	}
	if rotated.TokenHash != "new-token-hash" {
		t.Fatalf("expected new-token-hash, got %s", rotated.TokenHash)
	}

	got, err := repo.GetSession(ctx, session.ID)
	if err != nil || got == nil {
		t.Fatalf("GetSession after rotate: %v", err)
	}
	if got.TokenHash != "new-token-hash" {
		t.Fatalf("expected persisted new-token-hash, got %s", got.TokenHash)
	}
	if got.LastRefreshedAt == nil {
		t.Fatal("expected LastRefreshedAt to be set after rotation")
	}
}

func TestRotateSessionTokenMismatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	session := seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	// Rotate with wrong hash - family revoke should occur
	_, err := repo.RotateSession(ctx, session.ID, "wrong-hash", "new-token-hash", now)
	if err == nil {
		t.Fatal("expected error for token mismatch")
	}
	if !errors.Is(err, repository.ErrTokenMismatch) {
		t.Fatalf("expected ErrTokenMismatch, got %v", err)
	}

	// Session should be revoked as part of family revoke
	got, err := repo.GetSession(ctx, session.ID)
	if err != nil || got == nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != "revoked" {
		t.Fatalf("expected session to be revoked (family revoke), got status %s", got.Status)
	}
}

func TestRevokeSession(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	session := seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	if err := repo.RevokeSession(ctx, session.ID, "logout"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	got, err := repo.GetSession(ctx, session.ID)
	if err != nil || got == nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != "revoked" {
		t.Fatalf("expected status revoked, got %s", got.Status)
	}
}

func TestRotateSessionRevoked(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	session := seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	if err := repo.RevokeSession(ctx, session.ID, "logout"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	_, err := repo.RotateSession(ctx, session.ID, session.TokenHash, "new-token-hash", now)
	if err == nil {
		t.Fatal("expected error for revoked session")
	}
	if !errors.Is(err, repository.ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestRotateSessionIdleExpired(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	// Create session with idle expiry in the past
	session := seedSession(t, repo, ctx, now.Add(-1*time.Hour), absoluteExpiresAt)

	_, err := repo.RotateSession(ctx, session.ID, session.TokenHash, "new-token-hash", now)
	if err == nil {
		t.Fatal("expected error for idle-expired session")
	}
	if !errors.Is(err, repository.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestRotateSessionAbsoluteExpired(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	// Absolute expiry in the past
	session := seedSession(t, repo, ctx, now.Add(7*24*time.Hour), now.Add(-1*time.Hour))

	_, err := repo.RotateSession(ctx, session.ID, session.TokenHash, "new-token-hash", now)
	if err == nil {
		t.Fatal("expected error for absolutely expired session")
	}
	if !errors.Is(err, repository.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestRevokeUserSessions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)

	if err := repo.RevokeUserSessions(ctx, 1, "admin-action"); err != nil {
		t.Fatalf("RevokeUserSessions with no sessions: %v", err)
	}

	seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	if err := repo.RevokeUserSessions(ctx, 1, "admin-action"); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
}

func TestRotateSessionOnlyOneConcurrentWinner(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repository.NewAuthSessionRepo(db)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	absoluteExpiresAt := now.Add(30 * 24 * time.Hour)
	session := seedSession(t, repo, ctx, expiresAt, absoluteExpiresAt)

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.RotateSession(ctx, session.ID, session.TokenHash, "next-hash", time.Now())
			if err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("expected exactly 1 success, got %d", successes.Load())
	}
}
