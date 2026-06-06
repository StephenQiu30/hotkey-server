package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/authorization"
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
)

func TestAuthorizationService_Connect(t *testing.T) {
	enc := newTestEncryptor(t)
	authRepo := serviceauth.NewMemoryRepository()
	userSvc := newUserService(t, authRepo)
	azSvc := serviceauth.NewAuthorizationService(authRepo, nil, enc, time.Now)

	ctx := context.Background()

	// Register a user first
	account, err := userSvc.Register(ctx, serviceauth.RegisterInput{
		Email:    "auth-test@example.com",
		Password:  "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Connect GitHub authorization
	az, err := azSvc.Connect(ctx, serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.PlatformGitHub,
		PlatformUserID: "github-user-123",
		DisplayName:    "Test User",
		AccessToken:    "ghp_test_token_abc",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if az.Status != authorization.StatusConnected {
		t.Fatalf("expected status connected, got %s", az.Status)
	}
	if az.AccessTokenEnc == "ghp_test_token_abc" {
		t.Fatal("access token should be encrypted")
	}
}

func TestAuthorizationService_Disconnect(t *testing.T) {
	enc := newTestEncryptor(t)
	authRepo := serviceauth.NewMemoryRepository()
	userSvc := newUserService(t, authRepo)
	azSvc := serviceauth.NewAuthorizationService(authRepo, nil, enc, time.Now)

	ctx := context.Background()

	account, err := userSvc.Register(ctx, serviceauth.RegisterInput{
		Email:    "disconnect@example.com",
		Password:  "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	az, err := azSvc.Connect(ctx, serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.PlatformGitHub,
		PlatformUserID: "github-123",
		DisplayName:    "Test",
		AccessToken:    "ghp_token",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = azSvc.Disconnect(ctx, account.ID, az.ID)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Verify revoked
	list, err := azSvc.ListByUser(ctx, account.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 authorization, got %d", len(list))
	}
	if list[0].Status != authorization.StatusRevoked {
		t.Fatalf("expected revoked, got %s", list[0].Status)
	}
}

func TestAuthorizationService_HealthCheck_Expired(t *testing.T) {
	enc := newTestEncryptor(t)
	authRepo := serviceauth.NewMemoryRepository()
	userSvc := newUserService(t, authRepo)

	pastTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	azSvc := serviceauth.NewAuthorizationService(authRepo, nil, enc, func() time.Time { return pastTime })

	ctx := context.Background()

	account, err := userSvc.Register(ctx, serviceauth.RegisterInput{
		Email:    "health@example.com",
		Password:  "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Connect with expiration in the past relative to "now"
	expiredAt := pastTime.Add(-1 * time.Hour)
	az, err := azSvc.ConnectWithExpiry(ctx, serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.PlatformGitHub,
		PlatformUserID: "github-123",
		DisplayName:    "Test",
		AccessToken:    "ghp_token",
	}, &expiredAt)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Health check should detect expired
	result, err := azSvc.HealthCheck(ctx, account.ID, az.ID)
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if result.Status != authorization.StatusExpired {
		t.Fatalf("expected expired, got %s", result.Status)
	}
}

func TestAuthorizationService_ListByUser(t *testing.T) {
	enc := newTestEncryptor(t)
	authRepo := serviceauth.NewMemoryRepository()
	userSvc := newUserService(t, authRepo)
	azSvc := serviceauth.NewAuthorizationService(authRepo, nil, enc, time.Now)

	ctx := context.Background()

	account, err := userSvc.Register(ctx, serviceauth.RegisterInput{
		Email:    "list@example.com",
		Password:  "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Connect two platforms
	_, err = azSvc.Connect(ctx, serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.PlatformGitHub,
		PlatformUserID: "gh-1",
		DisplayName:    "GitHub",
		AccessToken:    "ghp_1",
	})
	if err != nil {
		t.Fatalf("connect github: %v", err)
	}

	_, err = azSvc.Connect(ctx, serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.PlatformRSS,
		PlatformUserID: "rss-1",
		DisplayName:    "RSS",
		AccessToken:    "rss_token",
	})
	if err != nil {
		t.Fatalf("connect rss: %v", err)
	}

	list, err := azSvc.ListByUser(ctx, account.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 authorizations, got %d", len(list))
	}
}

func TestAuthorizationService_DeleteAccount(t *testing.T) {
	enc := newTestEncryptor(t)
	authRepo := serviceauth.NewMemoryRepository()
	userSvc := newUserService(t, authRepo)
	azSvc := serviceauth.NewAuthorizationService(authRepo, nil, enc, time.Now)

	ctx := context.Background()

	account, err := userSvc.Register(ctx, serviceauth.RegisterInput{
		Email:    "delete@example.com",
		Password:  "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Login to create refresh token
	_, err = userSvc.Login(ctx, serviceauth.LoginInput{
		Email:    "delete@example.com",
		Password:  "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Connect authorization
	_, err = azSvc.Connect(ctx, serviceauth.ConnectInput{
		UserID:         account.ID,
		Platform:       authorization.PlatformGitHub,
		PlatformUserID: "gh-1",
		DisplayName:    "GitHub",
		AccessToken:    "ghp_token",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Delete account should revoke all authorizations and refresh tokens
	err = azSvc.DeleteAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}

	// Verify user is deleted
	_, err = userSvc.CurrentUser(ctx, account.ID)
	if err == nil {
		t.Fatal("expected error after user deletion, got nil")
	}

	// Verify all authorizations are deleted
	list, err := azSvc.ListByUser(ctx, account.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 authorizations after delete, got %d", len(list))
	}
}

func newTestEncryptor(t *testing.T) crypto.Encryptor {
	t.Helper()
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}
	return enc
}

func newUserService(t *testing.T, repo serviceauth.Repository) *serviceauth.Service {
	t.Helper()
	svc, err := serviceauth.NewService(repo, serviceauth.Config{
		AccessTokenSecret: "test-secret-key-for-testing-only!",
	})
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	return svc
}
