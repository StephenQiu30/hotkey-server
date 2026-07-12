package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
)

// Sentinel errors for auth operations.
var (
	AuthErrEmailExists        = errors.New("email already registered")
	AuthErrInvalidCredentials = errors.New("invalid email or password")
	AuthErrInvalidInput       = errors.New("invalid input")
	AuthErrAccountDisabled    = errors.New("account disabled")
)

// UserRepository defines the persistence interface for user operations.
type UserRepository interface {
	ExistsByEmail(ctx context.Context, email string) bool
	Create(ctx context.Context, email, passwordHash, displayName string) (dto.User, error)
	GetByEmail(ctx context.Context, email string) (*dto.User, error)
	GetByID(ctx context.Context, id int64) (*dto.User, error)
	UpdatePassword(ctx context.Context, userID int64, newPasswordHash string, now time.Time) error
	UpdateLastLogin(ctx context.Context, userID int64, now time.Time) error
	SetEmailVerified(ctx context.Context, userID int64, now time.Time) error
	Transaction(ctx context.Context, fn func(tx UserRepository) error) error
}

// TicketVerifier abstracts the verification ticket lifecycle.
type TicketVerifier interface {
	ClaimTicket(ctx context.Context, ticket string) (*TicketData, error)
	CompleteTicket(ctx context.Context, ticket string) error
	ReleaseTicket(ctx context.Context, ticket string) error
}

// SessionManager abstracts session creation, refresh, and revocation.
type SessionManager interface {
	Create(ctx context.Context, userID int64, ip, ua string) (*SessionTokens, error)
	Refresh(ctx context.Context, sessionID int64, currentRefreshToken string) (*SessionTokens, error)
	Logout(ctx context.Context, sessionID int64) error
	RevokeAll(ctx context.Context, userID int64) error
}

// VerificationManager abstracts the verification code lifecycle.
type VerificationManager interface {
	SendVerificationCode(ctx context.Context, input dto.VerificationSendInput) error
	ConfirmCode(ctx context.Context, input dto.VerificationConfirmInput) (string, error)
}

// AuthResult carries the result of an authentication operation.
type AuthResult struct {
	User   dto.User
	Tokens *SessionTokens
}

// AuthService provides authentication operations with verified registration.
type AuthService struct {
	repo     UserRepository
	verifier TicketVerifier
	sessions SessionManager
	mailer   Mailer
	verifMgr VerificationManager
	now      func() time.Time
	mu       sync.Mutex // protects sequential mock clock in tests
}

// SetNow overrides the default clock. Must be called before any business methods
// if a custom clock is required (testing). Exported for tests.
func (s *AuthService) SetNow(now func() time.Time) {
	s.now = now
}

// NewAuthServiceV2 creates a new AuthService with its dependencies.
func NewAuthServiceV2(repo UserRepository, verifier TicketVerifier, sessions SessionManager, mailer Mailer, verifMgr VerificationManager) *AuthService {
	return &AuthService{
		repo:     repo,
		verifier: verifier,
		sessions: sessions,
		mailer:   mailer,
		verifMgr: verifMgr,
		now:      time.Now,
	}
}

// RegisterVerified creates a new user account after successful verification.
// 1. Claim the verification ticket
// 2. DB transaction: create user + create session
// 3. Commit -> complete ticket
// 4. DB failure releases the ticket
func (s *AuthService) RegisterVerified(ctx context.Context, ticket, password, displayName string, ip, ua string) (*AuthResult, error) {
	// Validate password
	if err := security.ValidatePassword(password); err != nil {
		log.Printf("[auth] RegisterVerified: password validation failed: %v", err)
		return nil, AuthErrInvalidInput
	}

	// 1. Claim ticket
	ticketData, err := s.verifier.ClaimTicket(ctx, ticket)
	if err != nil {
		return nil, err
	}
	if ticketData.Purpose != enum.VerificationPurposeRegister {
		s.verifier.ReleaseTicket(ctx, ticket)
		log.Printf("[auth] RegisterVerified: ticket purpose mismatch: expected %s, got %s", enum.VerificationPurposeRegister, ticketData.Purpose)
		return nil, AuthErrInvalidInput
	}

	normalizedEmail, err := security.NormalizeEmail(ticketData.Email)
	if err != nil {
		s.verifier.ReleaseTicket(ctx, ticket)
		log.Printf("[auth] RegisterVerified: email normalization failed for ticket %s: %v", ticket[:8]+"...", err)
		return nil, AuthErrInvalidInput
	}

	// Hash password outside the transaction.
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		s.verifier.ReleaseTicket(ctx, ticket)
		return nil, err
	}

	var registeredUser dto.User

	// 2. DB transaction
	err = s.repo.Transaction(ctx, func(tx UserRepository) error {
		// Duplicate check
		if tx.ExistsByEmail(ctx, normalizedEmail) {
			return AuthErrEmailExists
		}

		user, err := tx.Create(ctx, normalizedEmail, passwordHash, displayName)
		if err != nil {
			// Likely unique constraint violation
			log.Printf("[auth] RegisterVerified: create user failed: %v", err)
			return AuthErrEmailExists
		}

		if err := tx.SetEmailVerified(ctx, user.ID, s.now()); err != nil {
			return err
		}

		registeredUser = user
		return nil
	})
	if err != nil {
		// 4. DB failure releases the ticket
		s.verifier.ReleaseTicket(ctx, ticket)
		return nil, err
	}

	// The user transaction must commit before the session repository can
	// satisfy its foreign key through its independently injected DB handle.
	sessionTokens, err := s.sessions.Create(ctx, registeredUser.ID, ip, ua)
	if err != nil {
		s.verifier.ReleaseTicket(ctx, ticket)
		return nil, err
	}
	result := &AuthResult{User: registeredUser, Tokens: sessionTokens}

	// 3. Commit -> complete ticket
	if completeErr := s.verifier.CompleteTicket(ctx, ticket); completeErr != nil {
		log.Printf("auth: failed to complete ticket after registration: %v", completeErr)
	}

	return result, nil
}

// Login authenticates a user by email and password.
func (s *AuthService) Login(ctx context.Context, email, password, ip, ua string) (*AuthResult, error) {
	normalized, err := security.NormalizeEmail(email)
	if err != nil {
		log.Printf("[auth] Login: email normalization failed for %q: %v", email, err)
		return nil, AuthErrInvalidCredentials
	}

	user, err := s.repo.GetByEmail(ctx, normalized)
	if err != nil || user == nil {
		// Dummy bcrypt compare to prevent user enumeration (constant-time)
		security.ComparePassword("$2a$10$invalidhash0000000000000000000000000000000000000000000", password)
		log.Printf("[auth] Login: user %q not found", normalized)
		return nil, AuthErrInvalidCredentials
	}

	// Constant-time password comparison
	if err := security.ComparePassword(user.PasswordHash, password); err != nil {
		log.Printf("[auth] Login: password mismatch for user %d", user.ID)
		return nil, AuthErrInvalidCredentials
	}

	// Verify account status
	if user.Status == string(enum.AccountStatusDisabled) {
		log.Printf("[auth] Login: account disabled for user %d", user.ID)
		return nil, AuthErrAccountDisabled
	}

	now := s.now()

	// Update last_login_at
	if err := s.repo.UpdateLastLogin(ctx, user.ID, now); err != nil {
		log.Printf("auth: failed to update last_login for user %d: %v", user.ID, err)
	}

	// Create session (nil-safe for legacy mode without SessionManager)
	result := &AuthResult{
		User: *user,
	}
	if s.sessions != nil {
		sessionTokens, err := s.sessions.Create(ctx, user.ID, ip, ua)
		if err != nil {
			return nil, err
		}
		result.Tokens = sessionTokens
	}

	return result, nil
}

// ResetPassword resets a user's password using a verification ticket.
func (s *AuthService) ResetPassword(ctx context.Context, ticket, newPassword string) error {
	if err := security.ValidatePassword(newPassword); err != nil {
		return AuthErrInvalidInput
	}

	// 1. Claim ticket
	ticketData, err := s.verifier.ClaimTicket(ctx, ticket)
	if err != nil {
		return err
	}
	if ticketData.Purpose != enum.VerificationPurposeResetPassword {
		s.verifier.ReleaseTicket(ctx, ticket)
		return AuthErrInvalidInput
	}

	normalizedEmail, err := security.NormalizeEmail(ticketData.Email)
	if err != nil {
		s.verifier.ReleaseTicket(ctx, ticket)
		return AuthErrInvalidInput
	}

	// Hash password outside the transaction.
	passwordHash, err := security.HashPassword(newPassword)
	if err != nil {
		s.verifier.ReleaseTicket(ctx, ticket)
		return err
	}

	now := s.now()
	var userEmail string

	// 2. DB transaction: update password + revoke all sessions
	err = s.repo.Transaction(ctx, func(tx UserRepository) error {
		user, err := tx.GetByEmail(ctx, normalizedEmail)
		if err != nil || user == nil {
			return AuthErrInvalidCredentials
		}
		userEmail = user.Email

		if err := tx.UpdatePassword(ctx, user.ID, passwordHash, now); err != nil {
			return err
		}

		// Revoke all sessions
		if err := s.sessions.RevokeAll(ctx, user.ID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		s.verifier.ReleaseTicket(ctx, ticket)
		return err
	}

	// 3. Commit -> complete ticket
	if completeErr := s.verifier.CompleteTicket(ctx, ticket); completeErr != nil {
		log.Printf("auth: failed to complete password-reset ticket: %v", completeErr)
	}

	// 4. Send notification email (after commit, non-blocking)
	go func() {
		notifyCtx := context.Background()
		subject := "[HotKey] Your password has been reset"
		body := "Your HotKey account password was successfully reset. If you did not request this change, please contact support immediately."
		if _, sendErr := s.mailer.Send(notifyCtx, userEmail, subject, body); sendErr != nil {
			log.Printf("auth: failed to send password-reset notification to %s: %v", userEmail[:3]+"***@"+userEmail[strings.LastIndex(userEmail, "@")+1:], sendErr)
		}
	}()

	return nil
}

// CurrentUser retrieves a user by their ID.
func (s *AuthService) CurrentUser(ctx context.Context, userID int64) (*dto.User, error) {
	return s.repo.GetByID(ctx, userID)
}

// ParseAccessToken parses and validates a JWT access token string.
func (s *AuthService) ParseAccessToken(tokenStr, secret, issuer, audience string) (*security.AccessClaims, error) {
	return security.ParseAccessToken(tokenStr, secret, issuer, audience)
}

// RefreshSession delegates to the underlying session manager.
func (s *AuthService) RefreshSession(ctx context.Context, sessionID int64, refreshToken string) (*SessionTokens, error) {
	if s.sessions == nil {
		return nil, errors.New("session management not configured")
	}
	return s.sessions.Refresh(ctx, sessionID, refreshToken)
}

// LogoutSession delegates to the underlying session manager.
func (s *AuthService) LogoutSession(ctx context.Context, sessionID int64) error {
	if s.sessions == nil {
		return nil
	}
	return s.sessions.Logout(ctx, sessionID)
}

// SendVerificationCode delegates to the underlying verification manager.
func (s *AuthService) SendVerificationCode(ctx context.Context, input dto.VerificationSendInput) error {
	if s.verifMgr == nil {
		return errors.New("verification manager not configured")
	}
	return s.verifMgr.SendVerificationCode(ctx, input)
}

// ConfirmCode delegates to the underlying verification manager and returns a ticket.
func (s *AuthService) ConfirmCode(ctx context.Context, input dto.VerificationConfirmInput) (string, error) {
	if s.verifMgr == nil {
		return "", errors.New("verification manager not configured")
	}
	return s.verifMgr.ConfirmCode(ctx, input)
}

// NewAuthService is kept for backward compatibility with existing wiring.
// It creates an AuthService with only a repository (legacy mode).
func NewAuthService(repo UserRepository) *AuthService {
	return &AuthService{
		repo: repo,
		now:  time.Now,
	}
}

// Register is a legacy convenience method that directly creates a user.
// It bypasses the verification-ticket flow. New code should prefer
// RegisterVerified.
func (s *AuthService) Register(ctx context.Context, input dto.RegisterInput) (dto.User, error) {
	if err := security.ValidatePassword(input.Password); err != nil {
		log.Printf("[auth] Register: password validation failed for %q: %v", input.Email, err)
		return dto.User{}, AuthErrInvalidInput
	}

	normalizedEmail, err := security.NormalizeEmail(input.Email)
	if err != nil {
		log.Printf("[auth] Register: email normalization failed for %q: %v", input.Email, err)
		return dto.User{}, AuthErrInvalidInput
	}

	if s.repo.ExistsByEmail(ctx, normalizedEmail) {
		return dto.User{}, AuthErrEmailExists
	}

	passwordHash, err := security.HashPassword(input.Password)
	if err != nil {
		return dto.User{}, err
	}

	return s.repo.Create(ctx, normalizedEmail, passwordHash, input.DisplayName)
}
