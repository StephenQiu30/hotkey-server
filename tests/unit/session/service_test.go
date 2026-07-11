package session_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	fakeauthsession "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/auth_session"
)

// ---------------------------------------------------------------------------
// Fake TokenManager — delegates to real security functions.
// ---------------------------------------------------------------------------

type fakeTokenManager struct{}

func (f *fakeTokenManager) SignAccessToken(sessionID int64) (string, error) {
	return security.SignAccessToken(security.AccessClaims{SessionID: sessionID}, "test-secret")
}

func (f *fakeTokenManager) NewRefreshToken() (string, string) {
	return security.NewRefreshToken()
}

func (f *fakeTokenManager) SHA256Digest(data string) string {
	return security.SHA256Digest(data)
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type harness struct {
	svc   *service.SessionService
	repo  *fakeauthsession.Repo
	clock time.Time // frozen clock; advance by reassigning
	mu    sync.Mutex
}

func newHarness(t *testing.T, startTime time.Time) *harness {
	t.Helper()

	repo := fakeauthsession.NewRepo()
	tokens := &fakeTokenManager{}

	h := &harness{
		repo:  repo,
		clock: startTime,
	}

	svc := service.NewSessionService(repo, tokens)
	svc.SetNow(func() time.Time {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.clock
	})

	h.svc = svc
	return h
}

// advance moves the harness clock forward by d.
func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(d)
}

// now returns the current harness time.
func (h *harness) now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clock
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func createSession(t *testing.T, h *harness, userID int64, ip, ua string) *service.SessionTokens {
	t.Helper()
	tokens, err := h.svc.Create(context.Background(), userID, ip, ua)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return tokens
}

func mustRefresh(t *testing.T, h *harness, sessionID int64, refreshToken string) *service.SessionTokens {
	t.Helper()
	tokens, err := h.svc.Refresh(context.Background(), sessionID, refreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	return tokens
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateReturnsValidTokens(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	if tokens.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
	if tokens.RefreshToken == "" {
		t.Fatal("expected non-empty refresh token")
	}

	// Access token should be a valid JWT.
	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.SessionID == 0 {
		t.Fatal("expected session ID in access token claims")
	}

	// Access token expiry: 15 minutes from now.
	expectedAccessExpiry := h.now().Add(15 * time.Minute)
	if !tokens.AccessExpiresAt.Equal(expectedAccessExpiry) {
		t.Fatalf("expected access expiry %v, got %v", expectedAccessExpiry, tokens.AccessExpiresAt)
	}

	// Refresh token expiry: 7 days from now.
	expectedRefreshExpiry := h.now().Add(7 * 24 * time.Hour)
	if !tokens.RefreshExpiresAt.Equal(expectedRefreshExpiry) {
		t.Fatalf("expected refresh expiry %v, got %v", expectedRefreshExpiry, tokens.RefreshExpiresAt)
	}
}

func TestRotationCreatesNewToken(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens1 := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens1.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// Advance clock by 1 hour, then refresh.
	h.advance(1 * time.Hour)
	tokens2 := mustRefresh(t, h, sessionID, tokens1.RefreshToken)

	// New refresh token should be different.
	if tokens2.RefreshToken == tokens1.RefreshToken {
		t.Fatal("expected different refresh token after refresh")
	}

	// Access token should have the same session ID (same session, re-signed).
	claims2, err := security.ParseAccessToken(tokens2.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken after refresh: %v", err)
	}
	if claims2.SessionID != sessionID {
		t.Fatalf("expected same session ID %d, got %d", sessionID, claims2.SessionID)
	}

	// Refresh expiry should have been extended by 7 days from the refresh time.
	expectedRefreshExpiry := h.now().Add(7 * 24 * time.Hour)
	if !tokens2.RefreshExpiresAt.Equal(expectedRefreshExpiry) {
		t.Fatalf("expected refresh expiry %v, got %v", expectedRefreshExpiry, tokens2.RefreshExpiresAt)
	}

	// Old refresh token should no longer work.
	_, err = h.svc.Refresh(context.Background(), sessionID, tokens1.RefreshToken)
	if !errors.Is(err, service.ErrTokenReused) {
		t.Fatalf("expected ErrTokenReused for old refresh token, got %v", err)
	}
}

func TestLogoutIdempotent(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// First logout succeeds.
	if err := h.svc.Logout(context.Background(), sessionID); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Second logout (idempotent) should also succeed.
	if err := h.svc.Logout(context.Background(), sessionID); err != nil {
		t.Fatalf("Logout (idempotent): %v", err)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// Advance past the 7-day idle expiry.
	h.advance(8 * 24 * time.Hour)

	_, err = h.svc.Refresh(context.Background(), sessionID, tokens.RefreshToken)
	if !errors.Is(err, service.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestRevokedSessionRejected(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// Logout to revoke.
	if err := h.svc.Logout(context.Background(), sessionID); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Refresh should fail.
	_, err = h.svc.Refresh(context.Background(), sessionID, tokens.RefreshToken)
	if !errors.Is(err, service.ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestOldTokenReuseRevokesFamily(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// Refresh once to get a new token.
	h.advance(1 * time.Hour)
	mustRefresh(t, h, sessionID, tokens.RefreshToken)

	// Now use the OLD refresh token again — should trigger family revoke.
	_, err = h.svc.Refresh(context.Background(), sessionID, tokens.RefreshToken)
	if !errors.Is(err, service.ErrTokenReused) {
		t.Fatalf("expected ErrTokenReused, got %v", err)
	}

	// The session should now be revoked.
	_, err = h.svc.Refresh(context.Background(), sessionID, tokens.RefreshToken)
	if !errors.Is(err, service.ErrTokenReused) && !errors.Is(err, service.ErrSessionRevoked) {
		t.Fatalf("expected ErrTokenReused or ErrSessionRevoked, got %v", err)
	}
}

func TestNoExtensionPastAbsoluteExpiry(t *testing.T) {
	// Create a session with a near-term absolute expiry (e.g. 2 days from now).
	// This simulates the case where refreshes extend idle but hit the absolute cap.
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	_ = claims.SessionID

	// Advance 25 days — beyond the 7-day idle but within the fake's refresh
	// extension logic, the absolute expiry at creation + 30d should block this.
	// Actually, with 8 days the idle window expires. Let's advance 25 days
	// and verify the absolute expiry check kicks in by seeding a session
	// with a very short absolute expiry.

	// Create a second session with a short absolute expiry.
	shortTokens, err := h.svc.Create(context.Background(), 1, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	shortClaims, err := security.ParseAccessToken(shortTokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	shortID := shortClaims.SessionID

	// Manually set the absolute expiry to be very soon.
	session, err := h.repo.GetSession(context.Background(), shortID)
	if err != nil || session == nil {
		t.Fatal("expected session to exist")
	}
	// Override absolute expiry in the fake repo.
	session.AbsoluteExpiresAt = now.Add(2 * 24 * time.Hour)
	session.ExpiresAt = now.Add(2 * 24 * time.Hour)

	// Advance 3 days (past the short absolute expiry).
	h.advance(3 * 24 * time.Hour)

	_, err = h.svc.Refresh(context.Background(), shortID, shortTokens.RefreshToken)
	if !errors.Is(err, service.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired for absolutely expired session, got %v", err)
	}
}

func TestNoExtensionPastAbsoluteExpiryAfterRefresh(t *testing.T) {
	// Scenario: create session, refresh once (extending idle by 7 days),
	// but absolute expiry prevents the extension.
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// Override the session to have a short absolute expiry that is within
	// the first 7-day window.
	session, err := h.repo.GetSession(context.Background(), sessionID)
	if err != nil || session == nil {
		t.Fatal("expected session to exist")
	}
	session.AbsoluteExpiresAt = now.Add(5 * 24 * time.Hour)

	// Advance 4 days — still within both idle and absolute.
	h.advance(4 * 24 * time.Hour)

	// Refresh should extend idle to now+7d = day 11, but absolute caps at day 5.
	tokens2 := mustRefresh(t, h, sessionID, tokens.RefreshToken)

	// The refresh expiry should be capped at the absolute expiry (day 5).
	expectedCapped := now.Add(5 * 24 * time.Hour)
	if !tokens2.RefreshExpiresAt.Equal(expectedCapped) {
		t.Fatalf("expected refresh expiry capped at absolute %v, got %v",
			expectedCapped, tokens2.RefreshExpiresAt)
	}

	// Advance 1 more day + 1 second (day 5 from start) — now past absolute expiry.
	h.advance(1*24*time.Hour + 1*time.Second)

	// Refresh should now fail because absolute expiry is reached.
	_, err = h.svc.Refresh(context.Background(), sessionID, tokens2.RefreshToken)
	if !errors.Is(err, service.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired at absolute expiry, got %v", err)
	}
}

func TestRevokeAllUserSessions(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	// Create two sessions for same user.
	tokens1 := createSession(t, h, 1, "127.0.0.1", "test-agent")
	tokens2 := createSession(t, h, 1, "127.0.0.2", "test-agent-2")

	claims1, _ := security.ParseAccessToken(tokens1.AccessToken, "test-secret")
	claims2, _ := security.ParseAccessToken(tokens2.AccessToken, "test-secret")

	// Revoke all sessions for user 1.
	if err := h.svc.RevokeAll(context.Background(), 1); err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}

	// Both sessions should be revoked.
	_, err := h.svc.Refresh(context.Background(), claims1.SessionID, tokens1.RefreshToken)
	if !errors.Is(err, service.ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}

	_, err = h.svc.Refresh(context.Background(), claims2.SessionID, tokens2.RefreshToken)
	if !errors.Is(err, service.ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}

	// RevokeAll on a user with no sessions should succeed (no-op).
	if err := h.svc.RevokeAll(context.Background(), 999); err != nil {
		t.Fatalf("RevokeAll on non-existent user: %v", err)
	}
}

func TestRaceFreeRotation(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	tokens := createSession(t, h, 1, "127.0.0.1", "test-agent")

	claims, err := security.ParseAccessToken(tokens.AccessToken, "test-secret")
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	sessionID := claims.SessionID

	// Advance slightly so refresh time is unique.
	h.advance(1 * time.Minute)

	// Concurrent refresh attempts with the same (valid) refresh token.
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.svc.Refresh(context.Background(), sessionID, tokens.RefreshToken)
			if err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	// Only one refresh should succeed; all others get ErrTokenReused.
	if successes.Load() != 1 {
		t.Fatalf("expected exactly 1 successful refresh, got %d", successes.Load())
	}
}

func TestRefreshRejectsSessionNotFound(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	// Try to refresh a non-existent session.
	_, err := h.svc.Refresh(context.Background(), 99999, "some-token")
	if !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestLogoutNonExistentSession(t *testing.T) {
	h := newHarness(t, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))

	// Logging out a non-existent session should return an error.
	err := h.svc.Logout(context.Background(), 99999)
	if !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
