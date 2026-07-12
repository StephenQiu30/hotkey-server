package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
)

// ---------------------------------------------------------------------------
// Sentinel errors — stable, exported for consumers (controller/test).
// ---------------------------------------------------------------------------

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionRevoked  = errors.New("session is revoked")
	ErrSessionExpired  = errors.New("session has expired")
	ErrTokenReused     = errors.New("token reuse detected — family revoked")
)

// ---------------------------------------------------------------------------
// Interfaces consumed by SessionService
// ---------------------------------------------------------------------------

// AuthSessionRepository defines the persistence contract for auth sessions.
type AuthSessionRepository interface {
	CreateSession(ctx context.Context, userID int64, tokenHash, familyHash, ip, ua string, expiresAt, absoluteExpiresAt time.Time) (entity.AuthSession, error)
	GetSession(ctx context.Context, sessionID int64) (*entity.AuthSession, error)
	RevokeSession(ctx context.Context, sessionID int64, reason string) error
	RevokeUserSessions(ctx context.Context, userID int64, reason string) error
	RotateSession(ctx context.Context, sessionID int64, currentTokenHash, newTokenHash string, now time.Time) (entity.AuthSession, error)
}

// TokenManager abstracts JWT signing and refresh-token generation for testability.
type TokenManager interface {
	SignAccessToken(userID, sessionID int64) (string, error)
	NewRefreshToken() (token string, hash string)
	SHA256Digest(data string) string
}

// ---------------------------------------------------------------------------
// TokenManager production implementation
// ---------------------------------------------------------------------------

type tokenManager struct {
	jwtSecret   string
	jwtIssuer   string
	jwtAudience string
}

// NewTokenManager creates a TokenManager backed by real crypto/JWT.
func NewTokenManager(jwtSecret, jwtIssuer, jwtAudience string) TokenManager {
	return &tokenManager{
		jwtSecret:   jwtSecret,
		jwtIssuer:   jwtIssuer,
		jwtAudience: jwtAudience,
	}
}

func (m *tokenManager) SignAccessToken(userID, sessionID int64) (string, error) {
	claims := security.AccessClaims{SessionID: sessionID}
	claims.Subject = strconv.FormatInt(userID, 10)
	return security.SignAccessToken(claims, m.jwtSecret, m.jwtIssuer, m.jwtAudience)
}

func (m *tokenManager) NewRefreshToken() (string, string) {
	return security.NewRefreshToken()
}

func (m *tokenManager) SHA256Digest(data string) string {
	return security.SHA256Digest(data)
}

// ---------------------------------------------------------------------------
// SessionTokens
// ---------------------------------------------------------------------------

// SessionTokens carries the result of a create or refresh operation.
type SessionTokens struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// ---------------------------------------------------------------------------
// Repository error matcher
// ---------------------------------------------------------------------------

// repoError returns true when the error matches one of the known repository
// sentinel strings. We compare by string to avoid a direct import of the
// repository package (which would create an import cycle since the repository
// package already imports the service package).
func repoError(err error, s string) bool {
	return err != nil && err.Error() == s
}

const (
	repoErrSessionNotFound = "auth session not found"
	repoErrSessionRevoked  = "auth session is revoked"
	repoErrSessionExpired  = "auth session has expired"
	repoErrTokenMismatch   = "auth session token hash mismatch"
)

// ---------------------------------------------------------------------------
// SessionService
// ---------------------------------------------------------------------------

// SessionService manages rotating refresh-token sessions.
type SessionService struct {
	repo   AuthSessionRepository
	tokens TokenManager
	now    func() time.Time
}

// NewSessionService creates a SessionService with the given dependencies.
// The clock defaults to time.Now; override with SetNow for tests.
func NewSessionService(repo AuthSessionRepository, tokens TokenManager) *SessionService {
	return &SessionService{
		repo:   repo,
		tokens: tokens,
		now:    time.Now,
	}
}

// SetNow replaces the default clock. Must be called before any business
// methods if a custom clock is required (testing).
func (s *SessionService) SetNow(now func() time.Time) {
	s.now = now
}

// Create initiates a new session for the given user, returning signed access
// and refresh tokens.
func (s *SessionService) Create(ctx context.Context, userID int64, ip, ua string) (*SessionTokens, error) {
	now := s.now()

	// Generate refresh token (raw + hash).
	rawRefresh, refreshHash := s.tokens.NewRefreshToken()

	// Derive a family identifier for rotation tracking.
	_, familyHash := s.tokens.NewRefreshToken()

	// Calculate expiry windows.
	idleExpiry := now.Add(7 * 24 * time.Hour)      // 7-day idle timeout
	absoluteExpiry := now.Add(30 * 24 * time.Hour) // 30-day absolute timeout

	// Persist the session.
	session, err := s.repo.CreateSession(ctx, userID, refreshHash, familyHash, ip, ua, idleExpiry, absoluteExpiry)
	if err != nil {
		return nil, err
	}

	// Sign an access token bound to this session.
	accessToken, err := s.tokens.SignAccessToken(userID, session.ID)
	if err != nil {
		return nil, err
	}

	return &SessionTokens{
		AccessToken:      accessToken,
		AccessExpiresAt:  now.Add(15 * time.Minute),
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: idleExpiry,
	}, nil
}

// Refresh rotates the refresh token and issues a new access token for the
// given session. If the provided refresh token has already been rotated
// (hash mismatch), the entire session family is revoked and ErrTokenReused
// is returned.
func (s *SessionService) Refresh(ctx context.Context, sessionID int64, currentRefreshToken string) (*SessionTokens, error) {
	now := s.now()

	// Hash the provided refresh token for comparison.
	tokenHash := s.tokens.SHA256Digest(currentRefreshToken)

	// Generate a new refresh token.
	newRaw, newHash := s.tokens.NewRefreshToken()

	// Atomically rotate the token in the repository.
	session, err := s.repo.RotateSession(ctx, sessionID, tokenHash, newHash, now)
	if err != nil {
		switch {
		case repoError(err, repoErrSessionNotFound):
			return nil, ErrSessionNotFound
		case repoError(err, repoErrSessionRevoked):
			return nil, ErrSessionRevoked
		case repoError(err, repoErrSessionExpired):
			return nil, ErrSessionExpired
		case repoError(err, repoErrTokenMismatch):
			return nil, ErrTokenReused
		default:
			return nil, err
		}
	}

	// Sign a new access token.
	accessToken, err := s.tokens.SignAccessToken(session.UserID, session.ID)
	if err != nil {
		return nil, err
	}

	return &SessionTokens{
		AccessToken:      accessToken,
		AccessExpiresAt:  now.Add(15 * time.Minute),
		RefreshToken:     newRaw,
		RefreshExpiresAt: session.ExpiresAt,
	}, nil
}

// Logout revokes the specified session with reason "logout".
func (s *SessionService) Logout(ctx context.Context, sessionID int64) error {
	err := s.repo.RevokeSession(ctx, sessionID, "logout")
	if repoError(err, repoErrSessionNotFound) {
		return ErrSessionNotFound
	}
	return err
}

// RevokeAll revokes all active sessions for the given user.
func (s *SessionService) RevokeAll(ctx context.Context, userID int64) error {
	return s.repo.RevokeUserSessions(ctx, userID, "admin-revoke")
}
