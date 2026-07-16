package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
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

	if _, _, err := sessions.Rotate(context.Background(), initial.TokenHash, &domain.RefreshToken{TokenHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ExpiresAt: session.RefreshExpiry(now.Add(2 * time.Minute)), CreatedAt: now.Add(2 * time.Minute)}, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrRefreshReplay) {
		t.Fatalf("replay Rotate() error = %v, want domain.ErrRefreshReplay", err)
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
		case errors.Is(err, domain.ErrRefreshReplay):
			replays++
		default:
			t.Fatalf("concurrent Rotate() error = %v", err)
		}
	}
	if successes != 1 || replays != 1 {
		t.Fatalf("concurrent rotation outcomes = %d success %d replay, want 1 each", successes, replays)
	}
}

func TestSessionRepositoryRejectsDisabledOrDeletedUserBeforeRefreshConsumption(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*testing.T, *database.Runtime, int64)
	}{
		{
			name: "disabled",
			mutate: func(t *testing.T, runtime *database.Runtime, userID int64) {
				t.Helper()
				if _, err := runtime.SQL.Exec(`UPDATE users SET status = 'disabled' WHERE id = $1`, userID); err != nil {
					t.Fatalf("disable user: %v", err)
				}
			},
		},
		{
			name: "soft deleted",
			mutate: func(t *testing.T, runtime *database.Runtime, userID int64) {
				t.Helper()
				if _, err := runtime.SQL.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1`, userID); err != nil {
					t.Fatalf("delete user: %v", err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newIdentityRuntime(t)
			users := NewUserRepository(runtime)
			sessions := NewSessionRepository(runtime)
			user := createIdentityUser(t, users, tt.name)
			now := time.Now().UTC().Truncate(time.Microsecond)
			session := newIdentitySession(user.ID, now)
			initial := &domain.RefreshToken{TokenHash: "3333333333333333333333333333333333333333333333333333333333333333", ExpiresAt: session.RefreshExpiry(now), CreatedAt: now}
			if err := sessions.Create(context.Background(), &session, initial); err != nil {
				t.Fatalf("Create(): %v", err)
			}
			tt.mutate(t, runtime, user.ID)

			replacementHash := "4444444444444444444444444444444444444444444444444444444444444444"
			if _, _, err := sessions.Rotate(context.Background(), initial.TokenHash, &domain.RefreshToken{TokenHash: replacementHash, ExpiresAt: session.RefreshExpiry(now.Add(time.Minute)), CreatedAt: now.Add(time.Minute)}, now.Add(time.Minute)); !errors.Is(err, domain.ErrRefreshInvalid) {
				t.Fatalf("Rotate() error = %v, want domain.ErrRefreshInvalid", err)
			}
			var replacements, consumed int
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM auth_refresh_tokens WHERE token_hash = $1`, replacementHash).Scan(&replacements); err != nil {
				t.Fatalf("count replacement tokens: %v", err)
			}
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM auth_refresh_tokens WHERE id = $1 AND used_at IS NOT NULL`, initial.ID).Scan(&consumed); err != nil {
				t.Fatalf("read initial refresh token: %v", err)
			}
			if replacements != 0 || consumed != 0 {
				t.Fatalf("disabled/deleted rotation created=%d consumed=%d, want both 0", replacements, consumed)
			}
		})
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

func TestSessionRepositoryValidateAccessSessionReturnsCurrentDatabaseSubject(t *testing.T) {
	runtime := newIdentityRuntime(t)
	users := NewUserRepository(runtime)
	sessions := NewSessionRepository(runtime)
	user := createIdentityUser(t, users, "access-subject")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session := newIdentitySession(user.ID, now)
	token := &domain.RefreshToken{
		TokenHash: "abababababababababababababababababababababababababababababababab",
		ExpiresAt: session.RefreshExpiry(now),
		CreatedAt: now,
	}
	if err := sessions.Create(context.Background(), &session, token); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE users SET role = 'editor' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("change user role directly: %v", err)
	}

	subject, err := sessions.ValidateAccessSession(context.Background(), session.ID, now)
	if err != nil {
		t.Fatalf("ValidateAccessSession(): %v", err)
	}
	if subject.UserID != user.ID || subject.SessionID != session.ID || subject.Role != domain.RoleEditor {
		t.Fatalf("ValidateAccessSession() = %#v, want current database identity subject", subject)
	}
}

func TestSessionRepositoryValidateAccessSessionRejectsInvalidStateWithoutSubject(t *testing.T) {
	for _, tt := range []struct {
		name      string
		createdAt time.Time
		mutate    func(*testing.T, *database.Runtime, *SessionRepository, int64, int64, time.Time)
	}{
		{
			name:      "revoked session",
			createdAt: time.Now().UTC().Truncate(time.Microsecond),
			mutate: func(t *testing.T, _ *database.Runtime, sessions *SessionRepository, _ int64, sessionID int64, now time.Time) {
				t.Helper()
				if err := sessions.RevokeSession(context.Background(), sessionID, "logout", now); err != nil {
					t.Fatalf("RevokeSession(): %v", err)
				}
			},
		},
		{
			name:      "absolute expiry",
			createdAt: time.Now().UTC().Add(-31 * 24 * time.Hour).Truncate(time.Microsecond),
			mutate:    func(*testing.T, *database.Runtime, *SessionRepository, int64, int64, time.Time) {},
		},
		{
			name:      "disabled user",
			createdAt: time.Now().UTC().Truncate(time.Microsecond),
			mutate: func(t *testing.T, runtime *database.Runtime, _ *SessionRepository, userID int64, _ int64, _ time.Time) {
				t.Helper()
				if _, err := runtime.SQL.Exec(`UPDATE users SET status = 'disabled' WHERE id = $1`, userID); err != nil {
					t.Fatalf("disable user: %v", err)
				}
			},
		},
		{
			name:      "soft deleted user",
			createdAt: time.Now().UTC().Truncate(time.Microsecond),
			mutate: func(t *testing.T, runtime *database.Runtime, _ *SessionRepository, userID int64, _ int64, _ time.Time) {
				t.Helper()
				if _, err := runtime.SQL.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1`, userID); err != nil {
					t.Fatalf("soft delete user: %v", err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newIdentityRuntime(t)
			users := NewUserRepository(runtime)
			sessions := NewSessionRepository(runtime)
			user := createIdentityUser(t, users, "invalid-access-"+tt.name)
			now := time.Now().UTC().Truncate(time.Microsecond)
			session := newIdentitySession(user.ID, tt.createdAt)
			token := &domain.RefreshToken{
				TokenHash: "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd",
				ExpiresAt: session.RefreshExpiry(tt.createdAt),
				CreatedAt: tt.createdAt,
			}
			if err := sessions.Create(context.Background(), &session, token); err != nil {
				t.Fatalf("Create(): %v", err)
			}
			tt.mutate(t, runtime, sessions, user.ID, session.ID, now)

			subject, err := sessions.ValidateAccessSession(context.Background(), session.ID, now)
			if subject != (domain.Subject{}) {
				t.Fatalf("ValidateAccessSession() subject = %#v, want no subject", subject)
			}
			var appError *sharederrors.AppError
			if !errors.As(err, &appError) || appError.Code != sharederrors.CodeSessionInvalid || appError.Message != "session invalid" {
				t.Fatalf("ValidateAccessSession() error = %v, want safe session-invalid error", err)
			}
		})
	}
}
