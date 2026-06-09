package xauth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/service/xauth"
)

// fakeTokenExchanger implements TokenExchanger for testing.
type fakeTokenExchanger struct {
	exchangeErr error
	refreshErr  error
	revokeErr   error
	fixedToken  xauth.TokenResult
}

func (f *fakeTokenExchanger) ExchangeCode(_ context.Context, _ string, _ string, _ xauth.Config) (xauth.TokenResult, error) {
	if f.exchangeErr != nil {
		return xauth.TokenResult{}, f.exchangeErr
	}
	if f.fixedToken.AccessToken != "" {
		return f.fixedToken, nil
	}
	return xauth.TokenResult{
		AccessToken:  "fake_access_token_" + time.Now().Format("150405"),
		RefreshToken: "fake_refresh_token_" + time.Now().Format("150405"),
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}, nil
}

func (f *fakeTokenExchanger) RefreshToken(_ context.Context, _ string, _ xauth.Config) (xauth.TokenResult, error) {
	if f.refreshErr != nil {
		return xauth.TokenResult{}, f.refreshErr
	}
	return xauth.TokenResult{
		AccessToken:  "refreshed_access_token_" + time.Now().Format("150405"),
		RefreshToken: "refreshed_refresh_token_" + time.Now().Format("150405"),
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}, nil
}

func (f *fakeTokenExchanger) RevokeToken(_ context.Context, _ string, _ xauth.Config) error {
	return f.revokeErr
}

func TestGenerateAuthURLReturnsValidURL(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID:    "test_client_id",
		RedirectURL: "http://localhost:8080/api/v1/admin/x/auth/callback",
		Scopes:      []string{"tweet.read", "users.read", "offline.access"},
	})

	result, err := svc.GenerateAuthURL(context.Background(), "test_state")
	if err != nil {
		t.Fatalf("generate auth url: %v", err)
	}
	if result.URL == "" {
		t.Fatalf("expected non-empty auth URL")
	}
	if result.CodeVerifier == "" {
		t.Fatalf("expected non-empty code verifier for PKCE")
	}
	if result.State != "test_state" {
		t.Fatalf("expected state %q, got %q", "test_state", result.State)
	}
}

func TestGenerateAuthURLIncludesPKCEChallenge(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID:    "test_client_id",
		RedirectURL: "http://localhost:8080/api/v1/admin/x/auth/callback",
		Scopes:      []string{"tweet.read", "users.read"},
	})

	result, err := svc.GenerateAuthURL(context.Background(), "state_pkce")
	if err != nil {
		t.Fatalf("generate auth url: %v", err)
	}
	// The code verifier should be stored for later token exchange.
	stored, err := repo.GetPendingState(context.Background(), "state_pkce")
	if err != nil {
		t.Fatalf("get pending state: %v", err)
	}
	if stored.CodeVerifier != result.CodeVerifier {
		t.Fatalf("expected stored code verifier to match returned value")
	}
}

func TestExchangeCodeForTokenSuccess(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewServiceWithExchanger(repo, xauth.Config{
		ClientID:    "test_client_id",
		RedirectURL: "http://localhost:8080/api/v1/admin/x/auth/callback",
		Scopes:      []string{"tweet.read", "users.read", "offline.access"},
	}, &fakeTokenExchanger{})

	// Setup: store a pending state with code verifier.
	_, err := svc.GenerateAuthURL(context.Background(), "state_exchange")
	if err != nil {
		t.Fatalf("generate auth url: %v", err)
	}

	// Exchange with a fake token endpoint (simulated via test transport).
	token, err := svc.ExchangeCode(context.Background(), xauth.ExchangeInput{
		Code:         "auth_code_123",
		State:        "state_exchange",
		SourceID:     "src_x_exchange",
		CodeVerifier: "", // Will be looked up from stored state
	})
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	if token.AccessToken == "" {
		t.Fatalf("expected non-empty access token")
	}
	if token.RefreshToken == "" {
		t.Fatalf("expected non-empty refresh token")
	}

	// Verify credential was stored.
	cred, err := svc.GetCredential(context.Background(), "src_x_exchange")
	if err != nil {
		t.Fatalf("get credential after exchange: %v", err)
	}
	if cred.AccessToken != token.AccessToken {
		t.Fatalf("expected stored access token to match returned token")
	}
}

func TestExchangeCodeRejectsInvalidState(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID:    "test_client_id",
		RedirectURL: "http://localhost:8080/api/v1/admin/x/auth/callback",
	})

	_, err := svc.ExchangeCode(context.Background(), xauth.ExchangeInput{
		Code:     "auth_code_123",
		State:    "nonexistent_state",
		SourceID: "src_x_invalid",
	})
	if !errors.Is(err, xauth.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
}

func TestExchangeCodeRejectsMismatchedState(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID:    "test_client_id",
		RedirectURL: "http://localhost:8080/api/v1/admin/x/auth/callback",
	})

	_, err := svc.GenerateAuthURL(context.Background(), "state_a")
	if err != nil {
		t.Fatalf("generate auth url: %v", err)
	}

	_, err = svc.ExchangeCode(context.Background(), xauth.ExchangeInput{
		Code:     "auth_code_123",
		SourceID: "src_x_mismatch",
		State:    "state_b", // different state
	})
	if !errors.Is(err, xauth.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for mismatched state, got %v", err)
	}
}

func TestRefreshTokenSuccess(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewServiceWithExchanger(repo, xauth.Config{
		ClientID: "test_client_id",
	}, &fakeTokenExchanger{})

	// Store a credential to refresh.
	err := repo.StoreCredential(context.Background(), xauth.Credential{
		SourceID:     "src_x_1",
		AccessToken:  "old_access_token",
		RefreshToken: "valid_refresh_token",
	})
	if err != nil {
		t.Fatalf("store credential: %v", err)
	}

	token, err := svc.RefreshToken(context.Background(), "src_x_1")
	if err != nil {
		t.Fatalf("refresh token: %v", err)
	}
	if token.AccessToken == "" || token.AccessToken == "old_access_token" {
		t.Fatalf("expected new access token, got %q", token.AccessToken)
	}
}

func TestRefreshTokenFailsWhenNoCredential(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID: "test_client_id",
	})

	_, err := svc.RefreshToken(context.Background(), "nonexistent_source")
	if !errors.Is(err, xauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRevokeCredentialClearsTokens(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewServiceWithExchanger(repo, xauth.Config{
		ClientID: "test_client_id",
	}, &fakeTokenExchanger{})

	err := repo.StoreCredential(context.Background(), xauth.Credential{
		SourceID:     "src_x_revoke",
		AccessToken:  "access_token",
		RefreshToken: "refresh_token",
	})
	if err != nil {
		t.Fatalf("store credential: %v", err)
	}

	err = svc.RevokeCredential(context.Background(), "src_x_revoke")
	if err != nil {
		t.Fatalf("revoke credential: %v", err)
	}

	_, err = repo.GetCredential(context.Background(), "src_x_revoke")
	if !errors.Is(err, xauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after revoke, got %v", err)
	}
}

func TestRevokeCredentialFailsWhenNotFound(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID: "test_client_id",
	})

	err := svc.RevokeCredential(context.Background(), "nonexistent")
	if !errors.Is(err, xauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetCredentialReturnsStoredCredential(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID: "test_client_id",
	})

	err := repo.StoreCredential(context.Background(), xauth.Credential{
		SourceID:     "src_x_get",
		AccessToken:  "my_access_token",
		RefreshToken: "my_refresh_token",
	})
	if err != nil {
		t.Fatalf("store credential: %v", err)
	}

	cred, err := svc.GetCredential(context.Background(), "src_x_get")
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	if cred.AccessToken != "my_access_token" {
		t.Fatalf("expected access token %q, got %q", "my_access_token", cred.AccessToken)
	}
}

func TestGetCredentialFailsWhenNotFound(t *testing.T) {
	repo := xauth.NewMemoryRepository()
	svc := xauth.NewService(repo, xauth.Config{
		ClientID: "test_client_id",
	})

	_, err := svc.GetCredential(context.Background(), "nonexistent")
	if !errors.Is(err, xauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
