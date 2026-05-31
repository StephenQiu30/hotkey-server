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
