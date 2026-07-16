// Package application owns the identity use cases and transaction boundaries.
package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/google/uuid"
)

const verificationCodeLifetime = 10 * time.Minute

// Dependencies are the application-layer identity ports. Infrastructure
// adapters are injected by bootstrap; this package never imports concrete
// PostgreSQL, Redis, SMTP, bcrypt, or JWT implementations.
type Dependencies struct {
	Runtime      *database.Runtime
	Users        domain.UserRepository
	Sessions     domain.SessionRepository
	Audit        domain.AuditRepository
	Passwords    domain.PasswordHasher
	Tokens       domain.TokenIssuer
	Verification domain.VerificationStore
	Mailer       domain.Mailer
	Clock        domain.Clock
}

// Service coordinates identity domain ports and owns every PostgreSQL write
// boundary through Runtime.WithinTransaction.
type Service struct {
	runtime      *database.Runtime
	users        domain.UserRepository
	sessions     domain.SessionRepository
	audit        domain.AuditRepository
	passwords    domain.PasswordHasher
	tokens       domain.TokenIssuer
	verification domain.VerificationStore
	mailer       domain.Mailer
	clock        domain.Clock
}

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Runtime == nil || dependencies.Users == nil || dependencies.Sessions == nil || dependencies.Audit == nil || dependencies.Passwords == nil || dependencies.Tokens == nil || dependencies.Verification == nil || dependencies.Mailer == nil || dependencies.Clock == nil {
		return nil, errors.New("identity application dependencies are required")
	}
	return &Service{
		runtime:      dependencies.Runtime,
		users:        dependencies.Users,
		sessions:     dependencies.Sessions,
		audit:        dependencies.Audit,
		passwords:    dependencies.Passwords,
		tokens:       dependencies.Tokens,
		verification: dependencies.Verification,
		mailer:       dependencies.Mailer,
		clock:        dependencies.Clock,
	}, nil
}

type RegisterInput struct {
	VerificationTicket string
	Password           string
	DisplayName        string
}

type Credentials struct {
	Email    string
	Password string
}

// Authentication contains the access token, one opaque refresh value for the
// transport to place in a cookie, and the safe current user record. It never
// exposes a password hash through HTTP DTOs.
type Authentication struct {
	AccessToken  string
	RefreshToken string
	User         domain.User
}

type UserUpdate struct {
	Role   *domain.Role
	Status *domain.UserStatus
}

// Authenticator is the narrow dependency later consumed by HTTP middleware.
// It accepts a bearer token and returns only database-backed identity facts.
type Authenticator struct {
	tokens   domain.TokenIssuer
	sessions domain.SessionRepository
	clock    domain.Clock
}

func NewAuthenticator(tokens domain.TokenIssuer, sessions domain.SessionRepository, clock domain.Clock) *Authenticator {
	return &Authenticator{tokens: tokens, sessions: sessions, clock: clock}
}

func (service *Service) Authenticator() *Authenticator {
	if service == nil {
		return NewAuthenticator(nil, nil, nil)
	}
	return NewAuthenticator(service.tokens, service.sessions, service.clock)
}

// ListUsers intentionally delegates the read-only query port. Admin
// authorization belongs to the Task 5/6 transport middleware boundary.
func (service *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	if service == nil || service.users == nil {
		return nil, unavailable(nil)
	}
	return service.users.ListUsers(ctx)
}

func (authenticator *Authenticator) Authenticate(ctx context.Context, rawAccessToken string) (domain.Subject, error) {
	if authenticator == nil || authenticator.tokens == nil || authenticator.sessions == nil || authenticator.clock == nil || strings.TrimSpace(rawAccessToken) == "" {
		return domain.Subject{}, unauthenticated()
	}
	claims, err := authenticator.tokens.Parse(strings.TrimSpace(rawAccessToken))
	if err != nil {
		return domain.Subject{}, unauthenticated()
	}
	subject, err := authenticator.sessions.ValidateAccessSession(ctx, claims.SessionID, authenticator.clock.Now().UTC())
	if err != nil {
		return domain.Subject{}, sessionError(err)
	}
	if subject.UserID != claims.UserID || subject.SessionID != claims.SessionID || !subject.Role.Valid() {
		return domain.Subject{}, domain.SessionInvalid()
	}
	return subject, nil
}

// RequestVerification creates a short-lived code only after mail delivery
// succeeds. This prevents a delivery failure from leaving usable verification
// state behind. The flow deliberately does not inspect users, so registered
// and unregistered email addresses have the same public acceptance result.
func (service *Service) RequestVerification(ctx context.Context, purpose domain.VerificationPurpose, email string) error {
	if service == nil || service.verification == nil || service.mailer == nil || service.clock == nil {
		return unavailable(nil)
	}
	if !purpose.Valid() {
		return domain.VerificationInvalid()
	}
	normalizedEmail, err := domain.NormalizeEmail(email)
	if err != nil {
		return domain.VerificationInvalid()
	}
	code, err := verificationCode()
	if err != nil {
		return unavailable(err)
	}
	if err := service.mailer.SendVerificationCode(ctx, purpose, normalizedEmail, code); err != nil {
		return unavailable(err)
	}
	if err := service.verification.CreateCode(ctx, purpose, normalizedEmail, code, service.now().Add(verificationCodeLifetime)); err != nil {
		var appError *sharederrors.AppError
		if asAppError(err, &appError) {
			return appError
		}
		return unavailable(err)
	}
	return nil
}

func (service *Service) ConfirmVerification(ctx context.Context, purpose domain.VerificationPurpose, email, code string) (domain.VerificationTicket, error) {
	if service == nil || service.verification == nil || !purpose.Valid() {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	ticket, err := service.verification.ConsumeCode(ctx, purpose, email, code)
	if err != nil {
		return domain.VerificationTicket{}, verificationError(err)
	}
	normalizedEmail, normalizeErr := domain.NormalizeEmail(email)
	if normalizeErr != nil || ticket.Purpose != purpose || ticket.Token == "" || ticket.Email != normalizedEmail {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	return ticket, nil
}

func (service *Service) Register(ctx context.Context, input RegisterInput) (*domain.User, error) {
	if service == nil || service.passwords == nil || service.verification == nil {
		return nil, unavailable(nil)
	}
	passwordHash, err := service.passwords.Hash(input.Password)
	if err != nil {
		return nil, validationError(err)
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		return nil, validationError(nil)
	}

	var user domain.User
	err = service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		ticket, err := service.verification.ConsumeTicket(ctx, domain.VerificationPurposeRegistration, input.VerificationTicket)
		if err != nil {
			return verificationError(err)
		}
		if ticket.Purpose != domain.VerificationPurposeRegistration || strings.TrimSpace(ticket.Token) == "" {
			return domain.VerificationInvalid()
		}
		normalizedEmail, err := domain.NormalizeEmail(ticket.Email)
		if err != nil {
			return domain.VerificationInvalid()
		}
		user = domain.User{
			Email:        normalizedEmail,
			PasswordHash: passwordHash,
			DisplayName:  strings.TrimSpace(input.DisplayName),
			Role:         domain.RoleViewer,
			Status:       domain.UserStatusActive,
		}
		if err := service.users.Create(ctx, &user); err != nil {
			return err
		}
		return service.audit.Create(ctx, auditEntry("system", 0, "identity.registration", "user", user.ID, "success", nil, map[string]any{"role": string(user.Role), "status": string(user.Status)}))
	})
	if err != nil {
		return nil, registrationError(err)
	}
	return &user, nil
}

func (service *Service) Login(ctx context.Context, credentials Credentials) (Authentication, error) {
	var result Authentication
	if service == nil || service.users == nil || service.sessions == nil || service.passwords == nil || service.tokens == nil {
		return result, unavailable(nil)
	}

	var credentialFailure bool
	err := service.withTransaction(ctx, func(ctx context.Context, tx database.Transaction) error {
		user, err := service.users.FindByEmail(ctx, credentials.Email)
		if err != nil {
			if !errors.Is(err, sharedrepository.ErrNotFound) {
				return err
			}
			credentialFailure = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.login", "identity", 0, "failure", nil, nil))
		}
		if !user.Active() || service.passwords.Compare(user.PasswordHash, credentials.Password) != nil {
			credentialFailure = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.login", "identity", 0, "failure", nil, nil))
		}
		now := service.now()
		session := domain.NewSession(user.ID, uuid.NewString(), now)
		rawRefresh, refreshHash, err := newRefreshToken()
		if err != nil {
			return unavailable(err)
		}
		refresh := &domain.RefreshToken{TokenHash: refreshHash, ExpiresAt: session.RefreshExpiry(now), CreatedAt: now}
		if err := service.sessions.Create(ctx, &session, refresh); err != nil {
			return err
		}
		accessToken, err := service.issueAccessToken(session.UserID, session.ID, now)
		if err != nil {
			return err
		}
		if err := service.users.TouchLogin(ctx, user.ID, now); err != nil {
			return err
		}
		if err := service.audit.Create(ctx, auditEntry("user", user.ID, "identity.login", "session", session.ID, "success", nil, nil)); err != nil {
			return err
		}
		result = Authentication{AccessToken: accessToken, RefreshToken: rawRefresh, User: *user}
		return nil
	})
	if err != nil {
		return Authentication{}, serviceError(err)
	}
	if credentialFailure {
		return Authentication{}, domain.InvalidCredentials()
	}
	return result, nil
}

func (service *Service) Refresh(ctx context.Context, rawRefreshToken string) (Authentication, error) {
	var result Authentication
	if service == nil || service.sessions == nil || service.users == nil || service.tokens == nil {
		return result, unavailable(nil)
	}
	if strings.TrimSpace(rawRefreshToken) == "" {
		return result, domain.SessionInvalid()
	}

	var replay bool
	err := service.withTransaction(ctx, func(ctx context.Context, tx database.Transaction) error {
		now := service.now()
		currentSession, _, err := service.sessions.FindByRefreshTokenHash(ctx, hashRefreshToken(rawRefreshToken))
		if err != nil {
			if errors.Is(err, sharedrepository.ErrNotFound) {
				return domain.SessionInvalid()
			}
			return err
		}
		rawReplacement, replacementHash, err := newRefreshToken()
		if err != nil {
			return unavailable(err)
		}
		replacement := &domain.RefreshToken{
			TokenHash: replacementHash,
			ExpiresAt: currentSession.RefreshExpiry(now),
			CreatedAt: now,
		}
		session, _, err := service.sessions.Rotate(ctx, hashRefreshToken(rawRefreshToken), replacement, now)
		if errors.Is(err, domain.ErrRefreshReplay) {
			replay = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.refresh_replay", "session", currentSession.ID, "failure", nil, nil))
		}
		if err != nil {
			if errors.Is(err, domain.ErrRefreshInvalid) {
				return domain.SessionInvalid()
			}
			return err
		}
		user, err := service.users.FindByID(ctx, session.UserID)
		if err != nil {
			if errors.Is(err, sharedrepository.ErrNotFound) {
				return domain.SessionInvalid()
			}
			return err
		}
		if !user.Active() {
			return domain.SessionInvalid()
		}
		accessToken, err := service.issueAccessToken(session.UserID, session.ID, now)
		if err != nil {
			return err
		}
		if err := service.audit.Create(ctx, auditEntry("user", user.ID, "identity.refresh", "session", session.ID, "success", nil, nil)); err != nil {
			return err
		}
		result = Authentication{AccessToken: accessToken, RefreshToken: rawReplacement, User: *user}
		return nil
	})
	if err != nil {
		return Authentication{}, sessionError(err)
	}
	if replay {
		return Authentication{}, domain.SessionInvalid()
	}
	return result, nil
}

// Logout revokes the session established by a valid access subject, or (when
// the access token is unavailable) a currently valid refresh token. Missing or
// stale credentials intentionally remain a successful no-op.
func (service *Service) Logout(ctx context.Context, subject *domain.Subject, rawRefreshToken string) error {
	if service == nil || service.sessions == nil || service.audit == nil {
		return unavailable(nil)
	}
	err := service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		now := service.now()
		if subject != nil && subject.SessionID > 0 && subject.UserID > 0 {
			if err := service.sessions.RevokeSession(ctx, subject.SessionID, "logout", now); err != nil {
				return err
			}
			return service.audit.Create(ctx, auditEntry("user", subject.UserID, "identity.logout", "session", subject.SessionID, "success", nil, nil))
		}
		if strings.TrimSpace(rawRefreshToken) == "" {
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.logout", "identity", 0, "failure", nil, nil))
		}
		session, token, err := service.sessions.FindByRefreshTokenHash(ctx, hashRefreshToken(rawRefreshToken))
		if err != nil {
			if !errors.Is(err, sharedrepository.ErrNotFound) {
				return err
			}
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.logout", "identity", 0, "failure", nil, nil))
		}
		if token.UsedAt != nil || token.RevokedAt != nil || !token.ExpiresAt.After(now) {
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.logout", "identity", 0, "failure", nil, nil))
		}
		validated, err := service.sessions.ValidateAccessSession(ctx, session.ID, now)
		if err != nil {
			if errors.Is(err, sharedrepository.ErrUnavailable) {
				return err
			}
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.logout", "identity", 0, "failure", nil, nil))
		}
		if err := service.sessions.RevokeSession(ctx, validated.SessionID, "logout", now); err != nil {
			return err
		}
		return service.audit.Create(ctx, auditEntry("user", validated.UserID, "identity.logout", "session", validated.SessionID, "success", nil, nil))
	})
	return serviceError(err)
}

func (service *Service) CurrentUser(ctx context.Context, subject domain.Subject) (*domain.User, error) {
	if service == nil || service.users == nil || subject.UserID <= 0 || !subject.Role.Valid() {
		return nil, domain.SessionInvalid()
	}
	user, err := service.users.FindByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrUnavailable) {
			return nil, unavailable(err)
		}
		return nil, domain.SessionInvalid()
	}
	if !user.Active() || user.Role != subject.Role {
		return nil, domain.SessionInvalid()
	}
	return user, nil
}

func (service *Service) ChangePassword(ctx context.Context, subject domain.Subject, currentPassword, newPassword string) error {
	if service == nil || service.users == nil || service.sessions == nil || service.passwords == nil || subject.UserID <= 0 {
		return unavailable(nil)
	}
	newHash, err := service.passwords.Hash(newPassword)
	if err != nil {
		return validationError(err)
	}
	var credentialFailure bool
	err = service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		user, err := service.users.FindByID(ctx, subject.UserID)
		if err != nil {
			if !errors.Is(err, sharedrepository.ErrNotFound) {
				return err
			}
			credentialFailure = true
			return service.audit.Create(ctx, auditEntry("user", subject.UserID, "identity.password_change", "user", subject.UserID, "failure", nil, nil))
		}
		if !user.Active() || service.passwords.Compare(user.PasswordHash, currentPassword) != nil {
			credentialFailure = true
			return service.audit.Create(ctx, auditEntry("user", subject.UserID, "identity.password_change", "user", subject.UserID, "failure", nil, nil))
		}
		now := service.now()
		if err := service.users.UpdatePassword(ctx, user.ID, newHash, now); err != nil {
			return err
		}
		if err := service.sessions.RevokeAllForUser(ctx, user.ID, "password_changed", now); err != nil {
			return err
		}
		return service.audit.Create(ctx, auditEntry("user", user.ID, "identity.password_change", "user", user.ID, "success", nil, nil))
	})
	if err != nil {
		return serviceError(err)
	}
	if credentialFailure {
		return domain.InvalidCredentials()
	}
	return nil
}

func (service *Service) ConfirmPasswordReset(ctx context.Context, verificationTicket, newPassword string) error {
	if service == nil || service.verification == nil || service.users == nil || service.sessions == nil || service.passwords == nil {
		return unavailable(nil)
	}
	newHash, err := service.passwords.Hash(newPassword)
	if err != nil {
		return validationError(err)
	}
	var resetFailure bool
	err = service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		ticket, err := service.verification.ConsumeTicket(ctx, domain.VerificationPurposePasswordReset, verificationTicket)
		if err != nil {
			var appError *sharederrors.AppError
			if asAppError(err, &appError) && appError.Code == sharederrors.CodeUnavailable {
				return appError
			}
			resetFailure = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.password_reset", "identity", 0, "failure", nil, nil))
		}
		if ticket.Purpose != domain.VerificationPurposePasswordReset {
			resetFailure = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.password_reset", "identity", 0, "failure", nil, nil))
		}
		user, err := service.users.FindByEmail(ctx, ticket.Email)
		if err != nil {
			if !errors.Is(err, sharedrepository.ErrNotFound) {
				return err
			}
			resetFailure = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.password_reset", "identity", 0, "failure", nil, nil))
		}
		if !user.Active() {
			resetFailure = true
			return service.audit.Create(ctx, auditEntry("anonymous", 0, "identity.password_reset", "identity", 0, "failure", nil, nil))
		}
		now := service.now()
		if err := service.users.UpdatePassword(ctx, user.ID, newHash, now); err != nil {
			if !errors.Is(err, sharedrepository.ErrNotFound) {
				return err
			}
			resetFailure = true
			return service.audit.Create(ctx, auditEntry("user", user.ID, "identity.password_reset", "user", user.ID, "failure", nil, nil))
		}
		if err := service.sessions.RevokeAllForUser(ctx, user.ID, "password_reset", now); err != nil {
			return err
		}
		return service.audit.Create(ctx, auditEntry("user", user.ID, "identity.password_reset", "user", user.ID, "success", nil, nil))
	})
	if err != nil {
		return verificationError(err)
	}
	if resetFailure {
		return domain.VerificationInvalid()
	}
	return nil
}

func (service *Service) UpdateUser(ctx context.Context, actor domain.Subject, userID int64, update UserUpdate) (*domain.User, error) {
	if update.Role == nil && update.Status == nil {
		return nil, validationError(nil)
	}
	if update.Role != nil && !update.Role.Valid() || update.Status != nil && !update.Status.Valid() {
		return nil, validationError(nil)
	}
	if err := service.requireAdmin(ctx, actor, "identity.user_update", userID); err != nil {
		return nil, err
	}

	var changed *domain.User
	err := service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		now := service.now()
		var err error
		if update.Role != nil {
			changed, err = service.users.ChangeRole(ctx, userID, *update.Role, now)
			if err != nil {
				return err
			}
		}
		if update.Status != nil {
			changed, err = service.users.ChangeStatus(ctx, userID, *update.Status, now)
			if err != nil {
				return err
			}
			if *update.Status == domain.UserStatusDisabled {
				if err := service.sessions.RevokeAllForUser(ctx, userID, "user_disabled", now); err != nil {
					return err
				}
			}
		}
		if changed == nil {
			return validationError(nil)
		}
		return service.audit.Create(ctx, auditEntry("user", actor.UserID, "identity.user_update", "user", changed.ID, "success", nil, map[string]any{"role": string(changed.Role), "status": string(changed.Status)}))
	})
	if err != nil {
		if auditErr := service.auditLifecycleFailure(ctx, actor, "identity.user_update", userID); auditErr != nil {
			return nil, serviceError(auditErr)
		}
		return nil, lifecycleError(err)
	}
	return changed, nil
}

func (service *Service) DeleteUser(ctx context.Context, actor domain.Subject, userID int64) (*domain.User, error) {
	if err := service.requireAdmin(ctx, actor, "identity.user_delete", userID); err != nil {
		return nil, err
	}
	var deleted *domain.User
	err := service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		now := service.now()
		var err error
		deleted, err = service.users.SoftDelete(ctx, userID, now)
		if err != nil {
			return err
		}
		if err := service.sessions.RevokeAllForUser(ctx, userID, "user_deleted", now); err != nil {
			return err
		}
		return service.audit.Create(ctx, auditEntry("user", actor.UserID, "identity.user_delete", "user", userID, "success", map[string]any{"role": string(deleted.Role), "status": string(deleted.Status)}, map[string]any{"deleted_at": "set"}))
	})
	if err != nil {
		if auditErr := service.auditLifecycleFailure(ctx, actor, "identity.user_delete", userID); auditErr != nil {
			return nil, serviceError(auditErr)
		}
		return nil, lifecycleError(err)
	}
	return deleted, nil
}

func (service *Service) RestoreUser(ctx context.Context, actor domain.Subject, userID int64) (*domain.User, error) {
	if err := service.requireAdmin(ctx, actor, "identity.user_restore", userID); err != nil {
		return nil, err
	}
	var restored *domain.User
	err := service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		now := service.now()
		var err error
		restored, err = service.users.RestoreDisabled(ctx, userID, now)
		if err != nil {
			return err
		}
		if err := service.sessions.RevokeAllForUser(ctx, userID, "user_restored", now); err != nil {
			return err
		}
		return service.audit.Create(ctx, auditEntry("user", actor.UserID, "identity.user_restore", "user", userID, "success", map[string]any{"deleted_at": "set"}, map[string]any{"status": string(restored.Status), "deleted_at": nil}))
	})
	if err != nil {
		if auditErr := service.auditLifecycleFailure(ctx, actor, "identity.user_restore", userID); auditErr != nil {
			return nil, serviceError(auditErr)
		}
		return nil, lifecycleError(err)
	}
	return restored, nil
}

func (service *Service) now() time.Time {
	if service != nil && service.clock != nil {
		return service.clock.Now().UTC()
	}
	return time.Now().UTC()
}

func (service *Service) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if service == nil || service.runtime == nil {
		return unavailable(nil)
	}
	return service.runtime.WithinTransaction(ctx, fn)
}

func (service *Service) issueAccessToken(userID, sessionID int64, now time.Time) (string, error) {
	if service == nil || service.tokens == nil {
		return "", unavailable(nil)
	}
	return service.tokens.Issue(domain.AccessTokenClaims{
		UserID:    userID,
		SessionID: sessionID,
		TokenID:   uuid.NewString(),
		IssuedAt:  now,
		NotBefore: now,
		ExpiresAt: now.Add(domain.AccessTokenLifetime),
	})
}

func (service *Service) requireAdmin(ctx context.Context, actor domain.Subject, action string, resourceID int64) error {
	if service == nil || service.audit == nil {
		return unavailable(nil)
	}
	if actor.UserID > 0 && actor.Role == domain.RoleAdmin {
		return nil
	}
	if err := service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		return service.audit.Create(ctx, auditEntry("user", actor.UserID, action, "user", resourceID, "denied", nil, nil))
	}); err != nil {
		return serviceError(err)
	}
	return forbidden()
}

// auditLifecycleFailure runs only after the business transaction has rolled
// back. Keeping it separate ensures a rejected multi-field change cannot
// commit a partial role/status mutation merely to preserve an audit record.
func (service *Service) auditLifecycleFailure(ctx context.Context, actor domain.Subject, action string, resourceID int64) error {
	return service.withTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		return service.audit.Create(ctx, auditEntry("user", actor.UserID, action, "user", resourceID, "failure", nil, nil))
	})
}

func auditEntry(actorType string, actorID int64, action, resourceType string, resourceID int64, result string, before, after map[string]any) domain.AuditEntry {
	return domain.AuditEntry{
		ActorType:    actorType,
		ActorID:      actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       result,
		BeforeData:   before,
		AfterData:    after,
	}
}

func newRefreshToken() (raw string, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(bytes)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:])
}

func unauthenticated() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeUnauthenticated, 401, "")
}

func forbidden() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeForbidden, 403, "")
}

func validationError(cause error) *sharederrors.AppError {
	_ = cause
	return sharederrors.New(sharederrors.CodeValidation, 400, "")
}

func conflictError(cause error) *sharederrors.AppError {
	_ = cause
	return sharederrors.New(sharederrors.CodeConflict, 409, "")
}

func notFoundError(cause error) *sharederrors.AppError {
	_ = cause
	return sharederrors.New(sharederrors.CodeNotFound, 404, "")
}

func verificationError(err error) error {
	var appError *sharederrors.AppError
	if asAppError(err, &appError) {
		return appError
	}
	return unavailable(err)
}

func registrationError(err error) error {
	if isVerificationError(err) {
		return verificationError(err)
	}
	return serviceError(err)
}

func lifecycleError(err error) error {
	var appError *sharederrors.AppError
	if asAppError(err, &appError) {
		return appError
	}
	if errors.Is(err, sharedrepository.ErrConflict) {
		return conflictError(err)
	}
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return notFoundError(err)
	}
	return serviceError(err)
}

func sessionError(err error) error {
	var appError *sharederrors.AppError
	if asAppError(err, &appError) {
		if appError.Code == sharederrors.CodeSessionInvalid || appError.Code == sharederrors.CodeUnavailable {
			return appError
		}
	}
	if errors.Is(err, sharedrepository.ErrUnavailable) {
		return unavailable(err)
	}
	return serviceError(err)
}

func serviceError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if asAppError(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, domain.ErrRefreshInvalid), errors.Is(err, domain.ErrRefreshReplay):
		return domain.SessionInvalid()
	case errors.Is(err, sharedrepository.ErrConflict):
		return conflictError(err)
	case errors.Is(err, sharedrepository.ErrNotFound):
		return notFoundError(err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return unavailable(err)
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return validationError(err)
	default:
		return sharederrors.New(sharederrors.CodeInternal, 500, "")
	}
}

func isVerificationError(err error) bool {
	var appError *sharederrors.AppError
	return asAppError(err, &appError) && (appError.Code == sharederrors.CodeVerificationInvalid || appError.Code == sharederrors.CodeUnavailable)
}

func verificationCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func asAppError(err error, target **sharederrors.AppError) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}

func unavailable(cause error) *sharederrors.AppError {
	_ = cause
	return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
}
