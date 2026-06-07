package xauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidState = errors.New("invalid or expired oauth state")
	ErrNotFound     = errors.New("credential not found")
	ErrInvalidInput = errors.New("invalid input")
)

// Config holds X OAuth configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// AuthURLResult contains the generated authorization URL and PKCE parameters.
type AuthURLResult struct {
	URL          string
	State        string
	CodeVerifier string
}

// TokenResult contains OAuth tokens from exchange or refresh.
type TokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Credential stores OAuth tokens for a source.
type Credential struct {
	SourceID     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PendingState stores an in-flight OAuth authorization.
type PendingState struct {
	State        string
	CodeVerifier string
	CreatedAt    time.Time
}

// Repository persists OAuth credentials and pending states.
type Repository interface {
	StorePendingState(ctx context.Context, state PendingState) error
	GetPendingState(ctx context.Context, state string) (PendingState, error)
	DeletePendingState(ctx context.Context, state string) error
	StoreCredential(ctx context.Context, cred Credential) error
	GetCredential(ctx context.Context, sourceID string) (Credential, error)
	DeleteCredential(ctx context.Context, sourceID string) error
}

// TokenExchanger abstracts the HTTP token exchange for testability.
type TokenExchanger interface {
	ExchangeCode(ctx context.Context, code string, codeVerifier string, config Config) (TokenResult, error)
	RefreshToken(ctx context.Context, refreshToken string, config Config) (TokenResult, error)
	RevokeToken(ctx context.Context, accessToken string, config Config) error
}

// Service manages X OAuth authorization flows.
type Service struct {
	repo      Repository
	exchanger TokenExchanger
	config    Config
	now       func() time.Time
}

// NewService creates a new X OAuth service.
func NewService(repo Repository, config Config) *Service {
	return &Service{
		repo:      repo,
		exchanger: &defaultTokenExchanger{},
		config:    config,
		now:       time.Now,
	}
}

// NewServiceWithExchanger creates a service with a custom token exchanger (for testing).
func NewServiceWithExchanger(repo Repository, config Config, exchanger TokenExchanger) *Service {
	return &Service{
		repo:      repo,
		exchanger: exchanger,
		config:    config,
		now:       time.Now,
	}
}

// GenerateAuthURL generates an X OAuth 2.0 authorization URL with PKCE.
func (s *Service) GenerateAuthURL(ctx context.Context, state string) (AuthURLResult, error) {
	if strings.TrimSpace(state) == "" {
		return AuthURLResult{}, ErrInvalidInput
	}

	codeVerifier := generateCodeVerifier()
	codeChallenge := generateCodeChallenge(codeVerifier)

	scopes := s.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"tweet.read", "users.read", "offline.access"}
	}

	authURL := fmt.Sprintf(
		"https://x.com/i/oauth2/authorize?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s&code_challenge=%s&code_challenge_method=S256",
		s.config.ClientID,
		s.config.RedirectURL,
		strings.Join(scopes, "%20"),
		state,
		codeChallenge,
	)

	if err := s.repo.StorePendingState(ctx, PendingState{
		State:        state,
		CodeVerifier: codeVerifier,
		CreatedAt:    s.now().UTC(),
	}); err != nil {
		return AuthURLResult{}, fmt.Errorf("store pending state: %w", err)
	}

	return AuthURLResult{
		URL:          authURL,
		State:        state,
		CodeVerifier: codeVerifier,
	}, nil
}

// ExchangeInput contains parameters for the OAuth code exchange.
type ExchangeInput struct {
	Code         string
	State        string
	SourceID     string // Required: associates credential with a source.
	CodeVerifier string // If empty, looked up from stored state.
}

// ExchangeCode exchanges an authorization code for tokens and stores the credential.
func (s *Service) ExchangeCode(ctx context.Context, input ExchangeInput) (TokenResult, error) {
	if strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.State) == "" || strings.TrimSpace(input.SourceID) == "" {
		return TokenResult{}, ErrInvalidInput
	}

	pending, err := s.repo.GetPendingState(ctx, input.State)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return TokenResult{}, ErrInvalidState
		}
		return TokenResult{}, err
	}

	codeVerifier := input.CodeVerifier
	if codeVerifier == "" {
		codeVerifier = pending.CodeVerifier
	}

	token, err := s.exchanger.ExchangeCode(ctx, input.Code, codeVerifier, s.config)
	if err != nil {
		return TokenResult{}, fmt.Errorf("exchange code: %w", err)
	}

	// Store the credential for future use (status, refresh, revoke).
	now := s.now().UTC()
	if err := s.repo.StoreCredential(ctx, Credential{
		SourceID:     strings.TrimSpace(input.SourceID),
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		return TokenResult{}, fmt.Errorf("store credential: %w", err)
	}

	// Clean up the pending state.
	_ = s.repo.DeletePendingState(ctx, input.State)

	return token, nil
}

// RefreshToken refreshes an OAuth token for a source.
func (s *Service) RefreshToken(ctx context.Context, sourceID string) (TokenResult, error) {
	if strings.TrimSpace(sourceID) == "" {
		return TokenResult{}, ErrInvalidInput
	}

	cred, err := s.repo.GetCredential(ctx, sourceID)
	if err != nil {
		return TokenResult{}, err
	}

	if strings.TrimSpace(cred.RefreshToken) == "" {
		return TokenResult{}, errors.New("no refresh token available")
	}

	token, err := s.exchanger.RefreshToken(ctx, cred.RefreshToken, s.config)
	if err != nil {
		return TokenResult{}, fmt.Errorf("refresh token: %w", err)
	}

	cred.AccessToken = token.AccessToken
	cred.RefreshToken = token.RefreshToken
	cred.ExpiresAt = token.ExpiresAt
	cred.UpdatedAt = s.now().UTC()

	if err := s.repo.StoreCredential(ctx, cred); err != nil {
		return TokenResult{}, fmt.Errorf("store refreshed credential: %w", err)
	}

	return token, nil
}

// RevokeCredential revokes and removes stored OAuth credentials for a source.
func (s *Service) RevokeCredential(ctx context.Context, sourceID string) error {
	if strings.TrimSpace(sourceID) == "" {
		return ErrInvalidInput
	}

	cred, err := s.repo.GetCredential(ctx, sourceID)
	if err != nil {
		return err
	}

	// Attempt to revoke the token on X's side; ignore errors as we still clear local state.
	_ = s.exchanger.RevokeToken(ctx, cred.AccessToken, s.config)

	return s.repo.DeleteCredential(ctx, sourceID)
}

// GetCredential retrieves stored OAuth credentials for a source.
func (s *Service) GetCredential(ctx context.Context, sourceID string) (Credential, error) {
	if strings.TrimSpace(sourceID) == "" {
		return Credential{}, ErrInvalidInput
	}
	return s.repo.GetCredential(ctx, sourceID)
}

// generateCodeVerifier generates a cryptographically random PKCE code verifier.
func generateCodeVerifier() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic(fmt.Sprintf("generate code verifier: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// generateCodeChallenge generates a PKCE code challenge from a verifier.
func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// defaultTokenExchanger implements TokenExchanger using HTTP calls to X API.
// This is a placeholder that will be configured with real credentials in production.
type defaultTokenExchanger struct{}

func (e *defaultTokenExchanger) ExchangeCode(_ context.Context, _ string, _ string, _ Config) (TokenResult, error) {
	return TokenResult{}, errors.New("x oauth token exchange not configured; set HOTKEY_X_CLIENT_ID and HOTKEY_X_CLIENT_SECRET")
}

func (e *defaultTokenExchanger) RefreshToken(_ context.Context, _ string, _ Config) (TokenResult, error) {
	return TokenResult{}, errors.New("x oauth token refresh not configured; set HOTKEY_X_CLIENT_ID and HOTKEY_X_CLIENT_SECRET")
}

func (e *defaultTokenExchanger) RevokeToken(_ context.Context, _ string, _ Config) error {
	return errors.New("x oauth token revoke not configured; set HOTKEY_X_CLIENT_ID and HOTKEY_X_CLIENT_SECRET")
}
