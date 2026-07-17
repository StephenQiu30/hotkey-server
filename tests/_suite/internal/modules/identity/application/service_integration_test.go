package application

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	identitypostgres "github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/security"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/internal/shared/requestcontext"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestServiceIntegrationUsesPostgresTransactionsForRefreshReplayAndUserRestoreConflict(t *testing.T) {
	runtime := openApplicationIntegrationRuntime(t)
	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	clock := fixedClock{now: now}
	issuer, err := security.NewJWT(security.JWTConfig{
		Secret:   "0123456789abcdef0123456789abcdef",
		Issuer:   "hotkey-test",
		Audience: "hotkey-web",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJWT() error = %v", err)
	}
	store := &verificationStoreFake{ticket: domain.VerificationTicket{
		Token:   "registration-one",
		Email:   "member@example.test",
		Purpose: domain.VerificationPurposeRegistration,
	}}
	service, err := NewService(Dependencies{
		Runtime:      runtime,
		Users:        identitypostgres.NewUserRepository(runtime),
		Sessions:     identitypostgres.NewSessionRepository(runtime),
		Audit:        identitypostgres.NewAuditRepository(runtime),
		Passwords:    security.NewPasswordHasher(),
		Tokens:       issuer,
		Verification: store,
		Mailer:       mailerFake{},
		Clock:        clock,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	original, err := service.Register(context.Background(), RegisterInput{
		VerificationTicket: "registration-one",
		Password:           "correct horse battery staple",
		DisplayName:        "Member",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	var preferences int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM user_preferences WHERE user_id = $1`, original.ID).Scan(&preferences); err != nil {
		t.Fatalf("count user preferences: %v", err)
	}
	if preferences != 1 || original.Role != domain.RoleViewer || original.Status != domain.UserStatusActive {
		t.Fatalf("registration persisted preferences=%d user=%#v, want active viewer plus preferences", preferences, original)
	}

	login, err := service.Login(context.Background(), Credentials{Email: original.Email, Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if _, err := service.Authenticator().Authenticate(context.Background(), login.AccessToken); err != nil {
		t.Fatalf("Authenticate(login access token) error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := service.Refresh(context.Background(), login.RefreshToken)
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var successes, invalid int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var appError *sharederrors.AppError
		if errors.As(err, &appError) && appError.Code == sharederrors.CodeSessionInvalid {
			invalid++
			continue
		}
		t.Fatalf("Refresh() error = %v", err)
	}
	if successes != 1 || invalid != 1 {
		t.Fatalf("Refresh() outcomes = %d success %d invalid, want one each", successes, invalid)
	}
	if _, err := service.Authenticator().Authenticate(context.Background(), login.AccessToken); err == nil {
		t.Fatal("replayed refresh left original access token authenticated")
	} else {
		requireAppCode(t, err, sharederrors.CodeSessionInvalid)
	}

	adminHasher := security.NewPasswordHasher()
	adminHash, err := adminHasher.Hash("admin password")
	if err != nil {
		t.Fatalf("Hash(admin password) error = %v", err)
	}
	admin := &domain.User{Email: "admin@example.test", PasswordHash: adminHash, DisplayName: "Admin", Role: domain.RoleAdmin, Status: domain.UserStatusActive}
	users := identitypostgres.NewUserRepository(runtime)
	if err := users.Create(context.Background(), admin); err != nil {
		t.Fatalf("Create(admin) error = %v", err)
	}
	actor := domain.Subject{UserID: admin.ID, Role: domain.RoleAdmin}
	if _, err := service.DeleteUser(context.Background(), actor, original.ID); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	store.ticket = domain.VerificationTicket{Token: "registration-two", Email: original.Email, Purpose: domain.VerificationPurposeRegistration}
	replacement, err := service.Register(context.Background(), RegisterInput{VerificationTicket: "registration-two", Password: "replacement password", DisplayName: "Replacement"})
	if err != nil {
		t.Fatalf("re-register after soft delete error = %v", err)
	}
	if replacement.ID == original.ID || replacement.Email != original.Email {
		t.Fatalf("replacement = %#v, want a new active account with same normalized email", replacement)
	}
	_, err = service.RestoreUser(context.Background(), actor, original.ID)
	requireAppCode(t, err, sharederrors.CodeConflict)
	if errors.Is(err, identitypostgres.ErrBootstrapUnavailable) || containsDatabaseDetail(err.Error()) {
		t.Fatalf("RestoreUser() leaked persistence detail: %v", err)
	}
	var originalDeleted, replacementActive bool
	if err := runtime.SQL.QueryRow(`SELECT deleted_at IS NOT NULL FROM users WHERE id = $1`, original.ID).Scan(&originalDeleted); err != nil {
		t.Fatalf("read original deleted state: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT deleted_at IS NULL AND status = 'active' FROM users WHERE id = $1`, replacement.ID).Scan(&replacementActive); err != nil {
		t.Fatalf("read replacement state: %v", err)
	}
	if !originalDeleted || !replacementActive {
		t.Fatalf("restore conflict mutated state: originalDeleted=%v replacementActive=%v", originalDeleted, replacementActive)
	}
}

func TestChangePasswordRevokesEveryRealPostgresSessionAndAccessJWT(t *testing.T) {
	fixture := newApplicationIntegrationFixture(t)
	user := fixture.register(t, "change-password@example.test", "current password")
	first, second := fixture.loginTwice(t, user.Email, "current password")
	subject, _ := fixture.assertTargetAccessAccepted(t, user, first.AccessToken, second.AccessToken)

	if err := fixture.service.ChangePassword(context.Background(), subject, "current password", "next password"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	fixture.assertAccessRejected(t, first.AccessToken)
	fixture.assertAccessRejected(t, second.AccessToken)
	fixture.assertAllUserSessionsRevoked(t, user.ID, 2)
	fixture.assertAudit(t, "identity.password_change", user.ID, "success")
}

func TestConfirmPasswordResetRevokesEveryRealPostgresSessionAndAccessJWT(t *testing.T) {
	fixture := newApplicationIntegrationFixture(t)
	user := fixture.register(t, "password-reset@example.test", "current password")
	first, second := fixture.loginTwice(t, user.Email, "current password")
	fixture.assertTargetAccessAccepted(t, user, first.AccessToken, second.AccessToken)
	fixture.store.ticket = domain.VerificationTicket{
		Token:   "password-reset-ticket",
		Email:   user.Email,
		Purpose: domain.VerificationPurposePasswordReset,
	}

	if err := fixture.service.ConfirmPasswordReset(context.Background(), "password-reset-ticket", "next password"); err != nil {
		t.Fatalf("ConfirmPasswordReset() error = %v", err)
	}
	fixture.assertAccessRejected(t, first.AccessToken)
	fixture.assertAccessRejected(t, second.AccessToken)
	fixture.assertAllUserSessionsRevoked(t, user.ID, 2)
	fixture.assertAudit(t, "identity.password_reset", user.ID, "success")
}

func TestAdminDisableRevokesTargetRealPostgresSessionsAndAccessJWT(t *testing.T) {
	fixture := newApplicationIntegrationFixture(t)
	target := fixture.register(t, "disable-target@example.test", "target password")
	first, second := fixture.loginTwice(t, target.Email, "target password")
	fixture.assertTargetAccessAccepted(t, target, first.AccessToken, second.AccessToken)
	admin := fixture.createAdminSubject(t, "disable-admin@example.test", "admin password")

	if _, err := fixture.service.UpdateUser(context.Background(), admin, target.ID, UserUpdate{Status: pointerToStatus(domain.UserStatusDisabled)}); err != nil {
		t.Fatalf("UpdateUser(disable) error = %v", err)
	}
	fixture.assertAccessRejected(t, first.AccessToken)
	fixture.assertAccessRejected(t, second.AccessToken)
	fixture.assertAllUserSessionsRevoked(t, target.ID, 2)
	fixture.assertAudit(t, "identity.user_update", target.ID, "success")
}

func TestAdminSoftDeleteRevokesTargetRealPostgresSessionsAndAccessJWT(t *testing.T) {
	fixture := newApplicationIntegrationFixture(t)
	target := fixture.register(t, "delete-target@example.test", "target password")
	first, second := fixture.loginTwice(t, target.Email, "target password")
	fixture.assertTargetAccessAccepted(t, target, first.AccessToken, second.AccessToken)
	admin := fixture.createAdminSubject(t, "delete-admin@example.test", "admin password")

	if _, err := fixture.service.DeleteUser(context.Background(), admin, target.ID); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	fixture.assertAccessRejected(t, first.AccessToken)
	fixture.assertAccessRejected(t, second.AccessToken)
	fixture.assertAllUserSessionsRevoked(t, target.ID, 2)
	fixture.assertAudit(t, "identity.user_delete", target.ID, "success")
	var deleted bool
	if err := fixture.runtime.SQL.QueryRow(`SELECT deleted_at IS NOT NULL FROM users WHERE id = $1`, target.ID).Scan(&deleted); err != nil {
		t.Fatalf("read soft-deleted target: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteUser() left target active in PostgreSQL")
	}
}

func TestServiceWritesSharedRequestAndTraceContextToRealAuditLog(t *testing.T) {
	fixture := newApplicationIntegrationFixture(t)
	user := fixture.register(t, "audit-context@example.test", "current password")
	ctx := requestcontext.WithTraceID(
		requestcontext.WithRequestID(context.Background(), "request-audit-77"),
		"4bf92f3577b34da6a3ce929d0e0e4736",
	)
	if _, err := fixture.service.Login(ctx, Credentials{Email: user.Email, Password: "current password"}); err != nil {
		t.Fatalf("Login(): %v", err)
	}

	var requestID, traceID sql.NullString
	if err := fixture.runtime.SQL.QueryRow(`
SELECT request_id, trace_id
FROM audit_logs
WHERE action = 'identity.login' AND actor_id = $1 AND result = 'success'
ORDER BY id DESC
LIMIT 1`, user.ID).Scan(&requestID, &traceID); err != nil {
		t.Fatalf("read identity login audit correlation: %v", err)
	}
	if !requestID.Valid || requestID.String != "request-audit-77" || !traceID.Valid || traceID.String != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("audit correlation request=%#v trace=%#v, want shared context values", requestID, traceID)
	}
}

type applicationIntegrationFixture struct {
	runtime *database.Runtime
	service *Service
	store   *verificationStoreFake
}

func newApplicationIntegrationFixture(t *testing.T) applicationIntegrationFixture {
	t.Helper()
	runtime := openApplicationIntegrationRuntime(t)
	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	issuer, err := security.NewJWT(security.JWTConfig{
		Secret:   "0123456789abcdef0123456789abcdef",
		Issuer:   "hotkey-test",
		Audience: "hotkey-web",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJWT() error = %v", err)
	}
	store := &verificationStoreFake{}
	service, err := NewService(Dependencies{
		Runtime:      runtime,
		Users:        identitypostgres.NewUserRepository(runtime),
		Sessions:     identitypostgres.NewSessionRepository(runtime),
		Audit:        identitypostgres.NewAuditRepository(runtime),
		Passwords:    security.NewPasswordHasher(),
		Tokens:       issuer,
		Verification: store,
		Mailer:       mailerFake{},
		Clock:        fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return applicationIntegrationFixture{runtime: runtime, service: service, store: store}
}

func (fixture applicationIntegrationFixture) register(t *testing.T, email, password string) *domain.User {
	t.Helper()
	fixture.store.ticket = domain.VerificationTicket{Token: "registration-" + email, Email: email, Purpose: domain.VerificationPurposeRegistration}
	user, err := fixture.service.Register(context.Background(), RegisterInput{VerificationTicket: fixture.store.ticket.Token, Password: password, DisplayName: "Integration User"})
	if err != nil {
		t.Fatalf("Register(%q) error = %v", email, err)
	}
	return user
}

func (fixture applicationIntegrationFixture) loginTwice(t *testing.T, email, password string) (Authentication, Authentication) {
	t.Helper()
	first, err := fixture.service.Login(context.Background(), Credentials{Email: email, Password: password})
	if err != nil {
		t.Fatalf("first Login(%q) error = %v", email, err)
	}
	second, err := fixture.service.Login(context.Background(), Credentials{Email: email, Password: password})
	if err != nil {
		t.Fatalf("second Login(%q) error = %v", email, err)
	}
	return first, second
}

func (fixture applicationIntegrationFixture) authenticate(t *testing.T, accessToken string) domain.Subject {
	t.Helper()
	subject, err := fixture.service.Authenticator().Authenticate(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	return subject
}

func (fixture applicationIntegrationFixture) assertTargetAccessAccepted(t *testing.T, target *domain.User, firstAccessToken, secondAccessToken string) (domain.Subject, domain.Subject) {
	t.Helper()
	first := fixture.authenticate(t, firstAccessToken)
	second := fixture.authenticate(t, secondAccessToken)
	for index, subject := range []domain.Subject{first, second} {
		if subject.UserID != target.ID || subject.Role != target.Role || subject.SessionID <= 0 {
			t.Fatalf("target access subject %d = %#v, want active user %d with role %q and a session", index+1, subject, target.ID, target.Role)
		}
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("target access subjects share session ID %d, want independently created sessions", first.SessionID)
	}
	return first, second
}

func (fixture applicationIntegrationFixture) createAdminSubject(t *testing.T, email, password string) domain.Subject {
	t.Helper()
	hasher := security.NewPasswordHasher()
	hash, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash(admin password) error = %v", err)
	}
	admin := &domain.User{Email: email, PasswordHash: hash, DisplayName: "Administrator", Role: domain.RoleAdmin, Status: domain.UserStatusActive}
	if err := identitypostgres.NewUserRepository(fixture.runtime).Create(context.Background(), admin); err != nil {
		t.Fatalf("Create(admin) error = %v", err)
	}
	login, err := fixture.service.Login(context.Background(), Credentials{Email: email, Password: password})
	if err != nil {
		t.Fatalf("Login(admin) error = %v", err)
	}
	return fixture.authenticate(t, login.AccessToken)
}

func (fixture applicationIntegrationFixture) assertAccessRejected(t *testing.T, accessToken string) {
	t.Helper()
	if _, err := fixture.service.Authenticator().Authenticate(context.Background(), accessToken); err == nil {
		t.Fatalf("Authenticator accepted revoked access token %q", accessToken)
	} else {
		requireAppCode(t, err, sharederrors.CodeSessionInvalid)
	}
}

func (fixture applicationIntegrationFixture) assertAllUserSessionsRevoked(t *testing.T, userID int64, wantTotal int) {
	t.Helper()
	var total, revoked, active int
	if err := fixture.runtime.SQL.QueryRow(`
SELECT count(*), count(*) FILTER (WHERE revoked_at IS NOT NULL), count(*) FILTER (WHERE revoked_at IS NULL)
FROM auth_sessions
WHERE user_id = $1`, userID).Scan(&total, &revoked, &active); err != nil {
		t.Fatalf("count auth sessions: %v", err)
	}
	if total != wantTotal || revoked != wantTotal || active != 0 {
		t.Fatalf("auth sessions total=%d revoked=%d active=%d, want %d/%d/0", total, revoked, active, wantTotal, wantTotal)
	}
}

func (fixture applicationIntegrationFixture) assertAudit(t *testing.T, action string, resourceID int64, result string) {
	t.Helper()
	var count int
	if err := fixture.runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = $1 AND resource_id = $2 AND result = $3`, action, resourceID, result).Scan(&count); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit action %q resource=%d result=%q count=%d, want 1", action, resourceID, result, count)
	}
}

func openApplicationIntegrationRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(context.Background(), postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(context.Background(), runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func containsDatabaseDetail(value string) bool {
	for _, fragment := range []string{"duplicate key", "users_active_email_uq", "pq:", "SQLSTATE"} {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
