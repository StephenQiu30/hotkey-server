package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
)

func TestSessionRepositoryRotatesLockedRefreshTokenAndRevokesSessionOnReplay(t *testing.T) {
	runtime := newIdentityRuntime(t)
	users := NewUserRepository(runtime)
	sessions := NewSessionRepository(runtime)
	user := createIdentityUser(t, users, "refresh")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session := newIdentitySession(user.ID, now)
	initial := &domain.RefreshToken{TokenHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: session.RefreshExpiry(now), CreatedAt: now}
	if err := sessions.Create(context.Background(), &session, initial); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	next := &domain.RefreshToken{TokenHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ExpiresAt: session.RefreshExpiry(now.Add(time.Minute)), CreatedAt: now.Add(time.Minute)}
	rotatedSession, rotatedToken, err := sessions.Rotate(context.Background(), initial.TokenHash, next, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Rotate(): %v", err)
	}
	if rotatedSession.ID != session.ID || rotatedToken.ID <= 0 || rotatedToken.SessionID != session.ID {
		t.Fatalf("Rotate() = session %#v token %#v, want persisted replacement", rotatedSession, rotatedToken)
	}

	if _, _, err := sessions.Rotate(context.Background(), initial.TokenHash, &domain.RefreshToken{TokenHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ExpiresAt: session.RefreshExpiry(now.Add(2 * time.Minute)), CreatedAt: now.Add(2 * time.Minute)}, now.Add(2*time.Minute)); !errors.Is(err, ErrRefreshReplay) {
		t.Fatalf("replay Rotate() error = %v, want ErrRefreshReplay", err)
	}

	var revokedAt *time.Time
	if err := runtime.SQL.QueryRow(`SELECT revoked_at FROM auth_sessions WHERE id = $1`, session.ID).Scan(&revokedAt); err != nil {
		t.Fatalf("read revoked session: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("session remains active after refresh replay")
	}
	var unrevoked int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM auth_refresh_tokens WHERE session_id = $1 AND revoked_at IS NULL`, session.ID).Scan(&unrevoked); err != nil {
		t.Fatalf("count active refresh tokens: %v", err)
	}
	if unrevoked != 0 {
		t.Fatalf("active refresh tokens = %d, want 0 after replay", unrevoked)
	}
}

func TestSessionRepositoryConcurrentConsumptionAllowsOnlyOneRotationThenRevokesReplay(t *testing.T) {
	runtime := newIdentityRuntime(t)
	users := NewUserRepository(runtime)
	sessions := NewSessionRepository(runtime)
	user := createIdentityUser(t, users, "concurrent-refresh")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session := newIdentitySession(user.ID, now)
	initial := &domain.RefreshToken{TokenHash: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", ExpiresAt: session.RefreshExpiry(now), CreatedAt: now}
	if err := sessions.Create(context.Background(), &session, initial); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, hash := range []string{
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	} {
		group.Add(1)
		go func(hash string) {
			defer group.Done()
			<-start
			_, _, err := sessions.Rotate(context.Background(), initial.TokenHash, &domain.RefreshToken{TokenHash: hash, ExpiresAt: session.RefreshExpiry(now.Add(time.Minute)), CreatedAt: now.Add(time.Minute)}, now.Add(time.Minute))
			errs <- err
		}(hash)
	}
	close(start)
	group.Wait()
	close(errs)

	var successes, replays int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRefreshReplay):
			replays++
		default:
			t.Fatalf("concurrent Rotate() error = %v", err)
		}
	}
	if successes != 1 || replays != 1 {
		t.Fatalf("concurrent rotation outcomes = %d success %d replay, want 1 each", successes, replays)
	}
}

func TestSessionRepositoryRevokesEverySessionForUser(t *testing.T) {
	runtime := newIdentityRuntime(t)
	users := NewUserRepository(runtime)
	sessions := NewSessionRepository(runtime)
	user := createIdentityUser(t, users, "revoke-all")
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, tokenHash := range []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
	} {
		session := newIdentitySession(user.ID, now)
		token := &domain.RefreshToken{TokenHash: tokenHash, ExpiresAt: session.RefreshExpiry(now), CreatedAt: now}
		if err := sessions.Create(context.Background(), &session, token); err != nil {
			t.Fatalf("Create(): %v", err)
		}
	}
	if err := sessions.RevokeAllForUser(context.Background(), user.ID, "password_changed", now.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeAllForUser(): %v", err)
	}
	var active int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NULL`, user.ID).Scan(&active); err != nil {
		t.Fatalf("count active sessions: %v", err)
	}
	if active != 0 {
		t.Fatalf("active sessions = %d, want 0", active)
	}
}
