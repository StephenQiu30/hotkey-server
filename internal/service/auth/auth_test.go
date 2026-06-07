package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	"github.com/StephenQiu30/hotkey-server/internal/service/auth"
)

func TestRegisterCreatesDefaultUserAndRejectsDuplicateEmail(t *testing.T) {
	service := newTestService(t)

	created, err := service.Register(context.Background(), auth.RegisterInput{
		Email:    "NewUser@Example.COM",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}
	if created.Email != "newuser@example.com" {
		t.Fatalf("expected normalized email, got %q", created.Email)
	}
	if created.Role != user.RoleUser {
		t.Fatalf("expected role %q, got %q", user.RoleUser, created.Role)
	}

	_, err = service.Register(context.Background(), auth.RegisterInput{
		Email:    "newuser@example.com",
		Password: "another correct horse",
	})
	if !errors.Is(err, auth.ErrEmailAlreadyExists) {
		t.Fatalf("expected duplicate email error, got %v", err)
	}
}

func TestLoginUsesUniformCredentialFailureAndIssuesRefreshToken(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	_, err := service.Register(ctx, auth.RegisterInput{
		Email:    "login@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	session, err := service.Login(ctx, auth.LoginInput{
		Email:    "login@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("expected access and refresh tokens, got %#v", session)
	}
	if session.User.Role != user.RoleUser {
		t.Fatalf("expected user role, got %q", session.User.Role)
	}

	_, err = service.Login(ctx, auth.LoginInput{
		Email:    "login@example.com",
		Password: "wrong password",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for wrong password, got %v", err)
	}

	_, err = service.Login(ctx, auth.LoginInput{
		Email:    "missing@example.com",
		Password: "wrong password",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected same invalid credentials for missing user, got %v", err)
	}
}

func TestNewServiceRejectsEmptyAccessTokenSecret(t *testing.T) {
	_, err := auth.NewService(auth.NewMemoryRepository(), auth.Config{})
	if err == nil {
		t.Fatal("expected empty access token secret to fail")
	}
}

func TestRefreshRejectsRevokedAndExpiredTokens(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	_, err := service.Register(ctx, auth.RegisterInput{
		Email:    "refresh@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}
	session, err := service.Login(ctx, auth.LoginInput{
		Email:    "refresh@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}

	refreshed, err := service.Refresh(ctx, session.RefreshToken)
	if err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}
	if refreshed.AccessToken == "" {
		t.Fatal("expected refreshed access token")
	}
	if refreshed.RefreshToken == "" || refreshed.RefreshToken == session.RefreshToken {
		t.Fatal("expected refresh to rotate the refresh token")
	}
	_, err = service.Refresh(ctx, session.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected rotated refresh token to reject replay, got %v", err)
	}

	if err := service.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("logout returned error: %v", err)
	}
	_, err = service.Refresh(ctx, refreshed.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected revoked refresh token to fail, got %v", err)
	}

	expiring := newTestServiceWithTTL(t, -time.Minute)
	_, err = expiring.Register(ctx, auth.RegisterInput{
		Email:    "expired@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}
	expiredSession, err := expiring.Login(ctx, auth.LoginInput{
		Email:    "expired@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	_, err = expiring.Refresh(ctx, expiredSession.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected expired refresh token to fail, got %v", err)
	}
}

func TestRevokeAllTokensForUserInvalidatesAllSessions(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()

	account, err := service.Register(ctx, auth.RegisterInput{
		Email:    "revoke-all@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Create two sessions
	s1, err := service.Login(ctx, auth.LoginInput{
		Email:    "revoke-all@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login 1: %v", err)
	}
	s2, err := service.Login(ctx, auth.LoginInput{
		Email:    "revoke-all@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login 2: %v", err)
	}

	// Both sessions should work initially; save the rotated tokens
	s1Refreshed, err := service.Refresh(ctx, s1.RefreshToken)
	if err != nil {
		t.Fatalf("refresh s1 before revoke: %v", err)
	}
	s2Refreshed, err := service.Refresh(ctx, s2.RefreshToken)
	if err != nil {
		t.Fatalf("refresh s2 before revoke: %v", err)
	}

	// Revoke all tokens for user
	if err := service.RevokeAllTokensForUser(ctx, account.ID); err != nil {
		t.Fatalf("revoke all: %v", err)
	}

	// Rotated tokens should be invalidated after revoke all
	_, err = service.Refresh(ctx, s1Refreshed.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected s1 rotated token invalidated after revoke all, got %v", err)
	}
	_, err = service.Refresh(ctx, s2Refreshed.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected s2 rotated token invalidated after revoke all, got %v", err)
	}
}

func TestDisableUserPreventsLoginAndRefresh(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()

	account, err := service.Register(ctx, auth.RegisterInput{
		Email:    "disable@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	session, err := service.Login(ctx, auth.LoginInput{
		Email:    "disable@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Disable the user
	if err := service.DisableUser(ctx, account.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}

	// Login should fail for disabled user
	_, err = service.Login(ctx, auth.LoginInput{
		Email:    "disable@example.com",
		Password: "correct horse battery staple",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for disabled user, got %v", err)
	}

	// Refresh should fail for disabled user (all tokens revoked)
	_, err = service.Refresh(ctx, session.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected invalid refresh token for disabled user, got %v", err)
	}
}

func newTestService(t *testing.T) *auth.Service {
	t.Helper()
	return newTestServiceWithTTL(t, 24*time.Hour)
}

func newTestServiceWithTTL(t *testing.T, refreshTTL time.Duration) *auth.Service {
	t.Helper()
	service, err := auth.NewService(auth.NewMemoryRepository(), auth.Config{
		AccessTokenSecret: "unit-test-secret",
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   refreshTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
