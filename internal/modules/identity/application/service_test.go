package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRequestVerificationDoesNotPersistCodeWhenSMTPFails(t *testing.T) {
	t.Parallel()

	store := &verificationStoreFake{}
	service := &Service{
		verification: store,
		mailer:       mailerFake{err: errors.New("smtp credentials rejected")},
		clock:        fixedClock{now: time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)},
	}

	err := service.RequestVerification(context.Background(), domain.VerificationPurposeRegistration, "member@example.test")
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != sharederrors.CodeUnavailable {
		t.Fatalf("RequestVerification() error = %v, want CodeUnavailable", err)
	}
	if store.createCalls != 0 {
		t.Fatalf("CreateCode calls = %d, want 0 when SMTP delivery fails", store.createCalls)
	}
}

func TestRequestVerificationHasTheSameAcceptedResultForExistingAndNewEmail(t *testing.T) {
	t.Parallel()

	clock := fixedClock{now: time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)}
	for _, email := range []string{"existing@example.test", "new@example.test"} {
		t.Run(email, func(t *testing.T) {
			store := &verificationStoreFake{}
			service := &Service{verification: store, mailer: mailerFake{}, clock: clock}

			if err := service.RequestVerification(context.Background(), domain.VerificationPurposeRegistration, email); err != nil {
				t.Fatalf("RequestVerification(%q) error = %v, want accepted result", email, err)
			}
			if store.createCalls != 1 || store.purpose != domain.VerificationPurposeRegistration || store.email != email {
				t.Fatalf("verification state = %#v, want one registration code for %q", store, email)
			}
		})
	}
}

func TestRegisterConsumesOnlyRegistrationTicketAndCreatesActiveViewer(t *testing.T) {
	service, users, store := newFakeService(t)
	store.ticket = domain.VerificationTicket{
		Token:   "registration-ticket",
		Email:   "new-member@example.test",
		Purpose: domain.VerificationPurposeRegistration,
	}

	user, err := service.Register(context.Background(), RegisterInput{
		VerificationTicket: "registration-ticket",
		Password:           "correct horse battery staple",
		DisplayName:        "New Member",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.Role != domain.RoleViewer || user.Status != domain.UserStatusActive || user.Email != "new-member@example.test" {
		t.Fatalf("Register() user = %#v, want normalized active viewer", user)
	}
	if store.consumeTicketPurpose != domain.VerificationPurposeRegistration || store.consumeTicketToken != "registration-ticket" {
		t.Fatalf("ConsumeTicket() = purpose %q token %q, want registration ticket", store.consumeTicketPurpose, store.consumeTicketToken)
	}
	if !users.lastWriteUsedTransaction {
		t.Fatal("Register() created user outside Runtime.WithinTransaction")
	}

	store.ticket = domain.VerificationTicket{Token: "reset-ticket", Email: "new-member@example.test", Purpose: domain.VerificationPurposePasswordReset}
	_, err = service.Register(context.Background(), RegisterInput{VerificationTicket: "reset-ticket", Password: "another password", DisplayName: "Nope"})
	requireAppCode(t, err, sharederrors.CodeVerificationInvalid)
}

func TestLoginDoesNotEnumerateCredentialsAndCreatesSessionInsideTransaction(t *testing.T) {
	service, users, _ := newFakeService(t)
	_, err := service.Login(context.Background(), Credentials{Email: "missing@example.test", Password: "wrong"})
	requireAppCode(t, err, sharederrors.CodeInvalidCredentials)

	users.put(domain.User{ID: 7, Email: "disabled@example.test", PasswordHash: "hash:password", Role: domain.RoleViewer, Status: domain.UserStatusDisabled})
	_, err = service.Login(context.Background(), Credentials{Email: "disabled@example.test", Password: "password"})
	requireAppCode(t, err, sharederrors.CodeInvalidCredentials)

	users.put(domain.User{ID: 8, Email: "member@example.test", PasswordHash: "hash:password", Role: domain.RoleViewer, Status: domain.UserStatusActive})
	result, err := service.Login(context.Background(), Credentials{Email: "member@example.test", Password: "password"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" || result.User.ID != 8 {
		t.Fatalf("Login() result = %#v, want access, opaque refresh, and user", result)
	}
	if !users.lastWriteUsedTransaction {
		t.Fatal("Login() wrote state outside Runtime.WithinTransaction")
	}
}

func TestLoginDoesNotTurnRepositoryOutageIntoInvalidCredentials(t *testing.T) {
	service, users, _ := newFakeService(t)
	users.findByEmailErr = sharedrepository.ErrUnavailable

	_, err := service.Login(context.Background(), Credentials{Email: "member@example.test", Password: "password"})
	requireAppCode(t, err, sharederrors.CodeUnavailable)
}

func TestAuthenticatorUsesCurrentDatabaseSubjectAndRejectsClaimMismatch(t *testing.T) {
	t.Parallel()

	clock := fixedClock{now: time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)}
	issuer := &tokenIssuerFake{parsed: map[string]domain.AccessTokenClaims{
		"valid":    {UserID: 11, SessionID: 21, TokenID: "token-1"},
		"mismatch": {UserID: 12, SessionID: 21, TokenID: "token-2"},
	}}
	sessions := &sessionRepositoryFake{subjects: map[int64]domain.Subject{
		21: {UserID: 11, SessionID: 21, Role: domain.RoleEditor},
	}, sessions: map[int64]domain.Session{
		21: {ID: 21, UserID: 11, AbsoluteExpiresAt: clock.now.Add(time.Hour)},
	}}
	authenticator := NewAuthenticator(issuer, sessions, clock)

	subject, err := authenticator.Authenticate(context.Background(), "valid")
	if err != nil {
		t.Fatalf("Authenticate(valid) error = %v", err)
	}
	if subject != (domain.Subject{UserID: 11, SessionID: 21, Role: domain.RoleEditor}) {
		t.Fatalf("Authenticate(valid) = %#v, want database role", subject)
	}
	if sessions.lastValidatedAt != clock.now {
		t.Fatalf("ValidateAccessSession() time = %s, want deterministic clock %s", sessions.lastValidatedAt, clock.now)
	}
	_, err = authenticator.Authenticate(context.Background(), "mismatch")
	requireAppCode(t, err, sharederrors.CodeSessionInvalid)
}

func TestRefreshHasOneWinnerThenReplayRevokesTheAccessSession(t *testing.T) {
	service, users, _ := newFakeService(t)
	users.put(domain.User{ID: 42, Email: "refresh@example.test", PasswordHash: "hash:password", Role: domain.RoleViewer, Status: domain.UserStatusActive})
	login, err := service.Login(context.Background(), Credentials{Email: "refresh@example.test", Password: "password"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, refreshErr := service.Refresh(context.Background(), login.RefreshToken)
			results <- refreshErr
		}()
	}
	close(start)
	var successes, invalid int
	for range 2 {
		err := <-results
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
		t.Fatal("Authenticate() accepted pre-replay access token")
	} else {
		requireAppCode(t, err, sharederrors.CodeSessionInvalid)
	}
}

func TestRefreshDoesNotTurnRepositoryOutageIntoSessionInvalid(t *testing.T) {
	service, _, _ := newFakeService(t)
	service.sessions.(*sessionRepositoryFake).findErr = sharedrepository.ErrUnavailable

	_, err := service.Refresh(context.Background(), "opaque-refresh")
	requireAppCode(t, err, sharederrors.CodeUnavailable)
}

func TestPasswordAndAdministratorLifecycleRevokeSessionsAndKeepSafeErrors(t *testing.T) {
	service, users, _ := newFakeService(t)
	users.put(domain.User{ID: 1, Email: "admin@example.test", PasswordHash: "hash:admin", Role: domain.RoleAdmin, Status: domain.UserStatusActive})
	users.put(domain.User{ID: 2, Email: "member@example.test", PasswordHash: "hash:old", Role: domain.RoleViewer, Status: domain.UserStatusActive})
	service.sessions.(*sessionRepositoryFake).putSession(domain.Session{ID: 22, UserID: 2, AbsoluteExpiresAt: service.now().Add(time.Hour)})

	if err := service.ChangePassword(context.Background(), domain.Subject{UserID: 2, SessionID: 22, Role: domain.RoleViewer}, "old", "new"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if got := users.user(2).PasswordHash; got != "hash:new" {
		t.Fatalf("password hash = %q, want changed hash", got)
	}
	if service.sessions.(*sessionRepositoryFake).activeSessionCount(2) != 0 {
		t.Fatal("ChangePassword() did not revoke all sessions")
	}

	_, err := service.UpdateUser(context.Background(), domain.Subject{UserID: 2, Role: domain.RoleViewer}, 1, UserUpdate{Role: pointerToRole(domain.RoleEditor)})
	requireAppCode(t, err, sharederrors.CodeForbidden)

	_, err = service.UpdateUser(context.Background(), domain.Subject{UserID: 1, Role: domain.RoleAdmin}, 1, UserUpdate{Status: pointerToStatus(domain.UserStatusDisabled)})
	requireAppCode(t, err, sharederrors.CodeLastActiveAdmin)

	if _, err := service.DeleteUser(context.Background(), domain.Subject{UserID: 1, Role: domain.RoleAdmin}, 2); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	users.put(domain.User{ID: 3, Email: "member@example.test", PasswordHash: "hash:replacement", Role: domain.RoleViewer, Status: domain.UserStatusActive})
	_, err = service.RestoreUser(context.Background(), domain.Subject{UserID: 1, Role: domain.RoleAdmin}, 2)
	requireAppCode(t, err, sharederrors.CodeConflict)
	if users.user(2).DeletedAt == nil || users.user(3).DeletedAt != nil {
		t.Fatalf("restore conflict mutated users: old=%#v replacement=%#v", users.user(2), users.user(3))
	}
}

func TestConfirmResetLogoutAndDisableUseTheCorrectIdentityState(t *testing.T) {
	service, users, store := newFakeService(t)
	users.put(domain.User{ID: 1, Email: "admin@example.test", PasswordHash: "hash:admin", Role: domain.RoleAdmin, Status: domain.UserStatusActive})
	users.put(domain.User{ID: 2, Email: "member@example.test", PasswordHash: "hash:old", Role: domain.RoleViewer, Status: domain.UserStatusActive})

	store.ticket = domain.VerificationTicket{Token: "confirmation-ticket", Email: "member@example.test", Purpose: domain.VerificationPurposeRegistration}
	ticket, err := service.ConfirmVerification(context.Background(), domain.VerificationPurposeRegistration, " MEMBER@example.test ", "123456")
	if err != nil || ticket.Token != "confirmation-ticket" {
		t.Fatalf("ConfirmVerification() = %#v, %v; want registration ticket", ticket, err)
	}
	if store.consumeCodePurpose != domain.VerificationPurposeRegistration {
		t.Fatalf("ConsumeCode purpose = %q, want registration", store.consumeCodePurpose)
	}

	login, err := service.Login(context.Background(), Credentials{Email: "member@example.test", Password: "old"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	store.ticket = domain.VerificationTicket{Token: "reset-ticket", Email: "member@example.test", Purpose: domain.VerificationPurposePasswordReset}
	if err := service.ConfirmPasswordReset(context.Background(), "reset-ticket", "new"); err != nil {
		t.Fatalf("ConfirmPasswordReset() error = %v", err)
	}
	if users.user(2).PasswordHash != "hash:new" || service.sessions.(*sessionRepositoryFake).activeSessionCount(2) != 0 {
		t.Fatal("password reset did not update credentials and revoke every session")
	}
	if _, err := service.Authenticator().Authenticate(context.Background(), login.AccessToken); err == nil {
		t.Fatal("password reset left previous access token valid")
	}

	secondLogin, err := service.Login(context.Background(), Credentials{Email: "member@example.test", Password: "new"})
	if err != nil {
		t.Fatalf("second Login() error = %v", err)
	}
	if err := service.Logout(context.Background(), nil, secondLogin.RefreshToken); err != nil {
		t.Fatalf("Logout(valid refresh) error = %v", err)
	}
	if _, err := service.Authenticator().Authenticate(context.Background(), secondLogin.AccessToken); err == nil {
		t.Fatal("refresh-based logout left access token valid")
	}
	if err := service.Logout(context.Background(), nil, "missing-refresh-token"); err != nil {
		t.Fatalf("Logout(missing refresh) error = %v, want idempotent success", err)
	}

	thirdLogin, err := service.Login(context.Background(), Credentials{Email: "member@example.test", Password: "new"})
	if err != nil {
		t.Fatalf("third Login() error = %v", err)
	}
	if _, err := service.UpdateUser(context.Background(), domain.Subject{UserID: 1, Role: domain.RoleAdmin}, 2, UserUpdate{Status: pointerToStatus(domain.UserStatusDisabled)}); err != nil {
		t.Fatalf("UpdateUser(disable) error = %v", err)
	}
	if service.sessions.(*sessionRepositoryFake).activeSessionCount(2) != 0 {
		t.Fatal("disabling user did not revoke every session")
	}
	if _, err := service.Authenticator().Authenticate(context.Background(), thirdLogin.AccessToken); err == nil {
		t.Fatal("disabling user left access token valid")
	}
}

func TestPasswordResetFailureWritesSanitizedAuditEvent(t *testing.T) {
	service, _, store := newFakeService(t)
	store.ticket = domain.VerificationTicket{Token: "registration-only", Email: "member@example.test", Purpose: domain.VerificationPurposeRegistration}

	err := service.ConfirmPasswordReset(context.Background(), "registration-only", "new password")
	requireAppCode(t, err, sharederrors.CodeVerificationInvalid)
	audit := service.audit.(*auditRepositoryFake)
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %#v, want one failed password-reset entry", audit.entries)
	}
	entry := audit.entries[0]
	if entry.Action != "identity.password_reset" || entry.Result != "failure" || entry.ActorType != "anonymous" || entry.BeforeData != nil || entry.AfterData != nil {
		t.Fatalf("audit entry = %#v, want sanitized failed reset audit", entry)
	}
}

func TestAdministratorInvariantFailureIsAuditedAfterItsTransactionRollsBack(t *testing.T) {
	service, users, _ := newFakeService(t)
	users.put(domain.User{ID: 1, Email: "admin@example.test", PasswordHash: "hash:admin", Role: domain.RoleAdmin, Status: domain.UserStatusActive})

	_, err := service.UpdateUser(context.Background(), domain.Subject{UserID: 1, Role: domain.RoleAdmin}, 1, UserUpdate{Status: pointerToStatus(domain.UserStatusDisabled)})
	requireAppCode(t, err, sharederrors.CodeLastActiveAdmin)
	audit := service.audit.(*auditRepositoryFake)
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %#v, want one lifecycle failure event", audit.entries)
	}
	entry := audit.entries[0]
	if entry.Action != "identity.user_update" || entry.Result != "failure" || entry.ActorID != 1 || entry.ResourceID != 1 || entry.BeforeData != nil || entry.AfterData != nil {
		t.Fatalf("audit entry = %#v, want sanitized failed admin update", entry)
	}
	if users.user(1).Status != domain.UserStatusActive {
		t.Fatalf("last admin state mutated to %q after rejected update", users.user(1).Status)
	}
}

func pointerToRole(role domain.Role) *domain.Role { return &role }

func pointerToStatus(status domain.UserStatus) *domain.UserStatus { return &status }

func requireAppCode(t *testing.T, err error, want int) {
	t.Helper()
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != want {
		t.Fatalf("error = %v, want application code %d", err, want)
	}
}

func newFakeService(t *testing.T) (*Service, *userRepositoryFake, *verificationStoreFake) {
	t.Helper()
	runtime := openTransactionRuntime(t)
	users := newUserRepositoryFake()
	sessions := newSessionRepositoryFake(users)
	store := &verificationStoreFake{}
	service, err := NewService(Dependencies{
		Runtime:      runtime,
		Users:        users,
		Sessions:     sessions,
		Audit:        &auditRepositoryFake{},
		Passwords:    passwordHasherFake{},
		Tokens:       &tokenIssuerFake{parsed: make(map[string]domain.AccessTokenClaims)},
		Verification: store,
		Mailer:       mailerFake{},
		Clock:        fixedClock{now: time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, users, store
}

func openTransactionRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(context.Background(), postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type mailerFake struct {
	err error
}

func (mailer mailerFake) SendVerificationCode(context.Context, domain.VerificationPurpose, string, string) error {
	return mailer.err
}

type verificationStoreFake struct {
	createCalls          int
	purpose              domain.VerificationPurpose
	email                string
	code                 string
	expiresAt            time.Time
	ticket               domain.VerificationTicket
	consumeCodePurpose   domain.VerificationPurpose
	consumeTicketPurpose domain.VerificationPurpose
	consumeTicketToken   string
	consumeTicketErr     error
}

func (store *verificationStoreFake) CreateCode(_ context.Context, purpose domain.VerificationPurpose, email, code string, expiresAt time.Time) error {
	store.createCalls++
	store.purpose = purpose
	store.email = email
	store.code = code
	store.expiresAt = expiresAt
	return nil
}

func (store *verificationStoreFake) ConsumeCode(_ context.Context, purpose domain.VerificationPurpose, _ string, _ string) (domain.VerificationTicket, error) {
	store.consumeCodePurpose = purpose
	if store.ticket.Purpose != purpose {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	return store.ticket, nil
}

func (store *verificationStoreFake) ConsumeTicket(_ context.Context, purpose domain.VerificationPurpose, token string) (domain.VerificationTicket, error) {
	store.consumeTicketPurpose = purpose
	store.consumeTicketToken = token
	if store.consumeTicketErr != nil {
		return domain.VerificationTicket{}, store.consumeTicketErr
	}
	if store.ticket.Token != token || store.ticket.Purpose != purpose {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	return store.ticket, nil
}

type passwordHasherFake struct{}

func (passwordHasherFake) Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}
	return "hash:" + password, nil
}

func (passwordHasherFake) Compare(hash, password string) error {
	if hash != "hash:"+password {
		return errors.New("password mismatch")
	}
	return nil
}

type tokenIssuerFake struct {
	mu     sync.Mutex
	parsed map[string]domain.AccessTokenClaims
	next   int
}

func (issuer *tokenIssuerFake) Issue(claims domain.AccessTokenClaims) (string, error) {
	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	issuer.next++
	raw := fmt.Sprintf("access-%d", issuer.next)
	issuer.parsed[raw] = claims
	return raw, nil
}

func (issuer *tokenIssuerFake) Parse(raw string) (domain.AccessTokenClaims, error) {
	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	claims, found := issuer.parsed[raw]
	if !found {
		return domain.AccessTokenClaims{}, errors.New("invalid access token")
	}
	return claims, nil
}

type userRepositoryFake struct {
	mu                       sync.Mutex
	users                    map[int64]domain.User
	nextID                   int64
	lastWriteUsedTransaction bool
	findByEmailErr           error
}

func newUserRepositoryFake() *userRepositoryFake {
	return &userRepositoryFake{users: make(map[int64]domain.User)}
}

func (repository *userRepositoryFake) put(user domain.User) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if user.ID > repository.nextID {
		repository.nextID = user.ID
	}
	repository.users[user.ID] = user
}

func (repository *userRepositoryFake) user(id int64) domain.User {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.users[id]
}

func (repository *userRepositoryFake) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if repository.findByEmailErr != nil {
		return nil, repository.findByEmailErr
	}
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, user := range repository.users {
		if user.DeletedAt == nil && user.Email == normalized {
			copy := user
			return &copy, nil
		}
	}
	return nil, sharedrepository.ErrNotFound
}

func (repository *userRepositoryFake) FindByID(_ context.Context, id int64) (*domain.User, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || user.DeletedAt != nil {
		return nil, sharedrepository.ErrNotFound
	}
	copy := user
	return &copy, nil
}

func (repository *userRepositoryFake) LockByID(ctx context.Context, id int64) (*domain.User, error) {
	if err := repository.requireTransaction(ctx); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found {
		return nil, sharedrepository.ErrNotFound
	}
	copy := user
	return &copy, nil
}

func (repository *userRepositoryFake) LockActiveAdmins(ctx context.Context) ([]domain.User, error) {
	if err := repository.requireTransaction(ctx); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	admins := make([]domain.User, 0)
	for _, user := range repository.users {
		if user.Active() && user.Role == domain.RoleAdmin {
			admins = append(admins, user)
		}
	}
	return admins, nil
}

func (repository *userRepositoryFake) Create(ctx context.Context, user *domain.User) error {
	if err := repository.requireTransaction(ctx); err != nil {
		return err
	}
	normalized, err := domain.NormalizeEmail(user.Email)
	if err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, existing := range repository.users {
		if existing.DeletedAt == nil && existing.Email == normalized {
			return sharedrepository.ErrConflict
		}
	}
	repository.nextID++
	user.ID = repository.nextID
	user.Email = normalized
	repository.users[user.ID] = *user
	return nil
}

func (repository *userRepositoryFake) UpdatePassword(ctx context.Context, id int64, hash string, _ time.Time) error {
	if err := repository.requireTransaction(ctx); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || !user.Active() {
		return sharedrepository.ErrNotFound
	}
	user.PasswordHash = hash
	repository.users[id] = user
	return nil
}

func (repository *userRepositoryFake) TouchLogin(ctx context.Context, id int64, now time.Time) error {
	if err := repository.requireTransaction(ctx); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || !user.Active() {
		return sharedrepository.ErrNotFound
	}
	value := now
	user.LastLoginAt = &value
	repository.users[id] = user
	return nil
}

func (repository *userRepositoryFake) ChangeRole(ctx context.Context, id int64, role domain.Role, _ time.Time) (*domain.User, error) {
	if err := repository.requireTransaction(ctx); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || user.DeletedAt != nil {
		return nil, sharedrepository.ErrNotFound
	}
	if removesLastAdmin(repository.users, user, role, user.Status, false) {
		return nil, domain.LastActiveAdmin()
	}
	user.Role = role
	repository.users[id] = user
	return &user, nil
}

func (repository *userRepositoryFake) ChangeStatus(ctx context.Context, id int64, status domain.UserStatus, _ time.Time) (*domain.User, error) {
	if err := repository.requireTransaction(ctx); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || user.DeletedAt != nil {
		return nil, sharedrepository.ErrNotFound
	}
	if removesLastAdmin(repository.users, user, user.Role, status, false) {
		return nil, domain.LastActiveAdmin()
	}
	user.Status = status
	repository.users[id] = user
	return &user, nil
}

func (repository *userRepositoryFake) SoftDelete(ctx context.Context, id int64, now time.Time) (*domain.User, error) {
	if err := repository.requireTransaction(ctx); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || user.DeletedAt != nil {
		return nil, sharedrepository.ErrNotFound
	}
	if removesLastAdmin(repository.users, user, user.Role, user.Status, true) {
		return nil, domain.LastActiveAdmin()
	}
	value := now
	user.DeletedAt = &value
	repository.users[id] = user
	return &user, nil
}

func (repository *userRepositoryFake) RestoreDisabled(ctx context.Context, id int64, _ time.Time) (*domain.User, error) {
	if err := repository.requireTransaction(ctx); err != nil {
		return nil, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	user, found := repository.users[id]
	if !found || user.DeletedAt == nil {
		return nil, sharedrepository.ErrNotFound
	}
	for otherID, other := range repository.users {
		if otherID != id && other.DeletedAt == nil && other.Email == user.Email {
			return nil, sharedrepository.ErrConflict
		}
	}
	user.DeletedAt = nil
	user.Status = domain.UserStatusDisabled
	repository.users[id] = user
	return &user, nil
}

func (repository *userRepositoryFake) requireTransaction(ctx context.Context) error {
	if _, found := database.TransactionFromContext(ctx); !found {
		return errors.New("identity write did not use runtime transaction")
	}
	repository.lastWriteUsedTransaction = true
	return nil
}

func removesLastAdmin(users map[int64]domain.User, target domain.User, role domain.Role, status domain.UserStatus, deleted bool) bool {
	if !target.Active() || target.Role != domain.RoleAdmin || (!deleted && role == domain.RoleAdmin && status == domain.UserStatusActive) {
		return false
	}
	count := 0
	for _, user := range users {
		if user.Active() && user.Role == domain.RoleAdmin {
			count++
		}
	}
	return count == 1
}

type sessionRepositoryFake struct {
	mu              sync.Mutex
	users           *userRepositoryFake
	sessions        map[int64]domain.Session
	tokens          map[string]domain.RefreshToken
	subjects        map[int64]domain.Subject
	nextSessionID   int64
	nextTokenID     int64
	lastValidatedAt time.Time
	findErr         error
}

func newSessionRepositoryFake(users *userRepositoryFake) *sessionRepositoryFake {
	return &sessionRepositoryFake{
		users:    users,
		sessions: make(map[int64]domain.Session),
		tokens:   make(map[string]domain.RefreshToken),
		subjects: make(map[int64]domain.Subject),
	}
}

func (repository *sessionRepositoryFake) putSession(session domain.Session) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if session.ID > repository.nextSessionID {
		repository.nextSessionID = session.ID
	}
	repository.sessions[session.ID] = session
	user := repository.users.user(session.UserID)
	repository.subjects[session.ID] = domain.Subject{UserID: session.UserID, SessionID: session.ID, Role: user.Role}
}

func (repository *sessionRepositoryFake) Create(ctx context.Context, session *domain.Session, token *domain.RefreshToken) error {
	if _, found := database.TransactionFromContext(ctx); !found {
		return errors.New("session write did not use runtime transaction")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.nextSessionID++
	session.ID = repository.nextSessionID
	repository.nextTokenID++
	token.ID = repository.nextTokenID
	token.SessionID = session.ID
	repository.sessions[session.ID] = *session
	repository.tokens[token.TokenHash] = *token
	user := repository.users.user(session.UserID)
	repository.subjects[session.ID] = domain.Subject{UserID: session.UserID, SessionID: session.ID, Role: user.Role}
	return nil
}

func (repository *sessionRepositoryFake) FindByRefreshTokenHash(_ context.Context, hash string) (*domain.Session, *domain.RefreshToken, error) {
	if repository.findErr != nil {
		return nil, nil, repository.findErr
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	token, found := repository.tokens[hash]
	if !found {
		return nil, nil, sharedrepository.ErrNotFound
	}
	session, found := repository.sessions[token.SessionID]
	if !found {
		return nil, nil, sharedrepository.ErrNotFound
	}
	return copySession(&session), copyToken(&token), nil
}

func (repository *sessionRepositoryFake) Rotate(ctx context.Context, currentHash string, replacement *domain.RefreshToken, now time.Time) (*domain.Session, *domain.RefreshToken, error) {
	if _, found := database.TransactionFromContext(ctx); !found {
		return nil, nil, errors.New("rotation did not use runtime transaction")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	token, found := repository.tokens[currentHash]
	if !found {
		return nil, nil, domain.ErrRefreshInvalid
	}
	session, found := repository.sessions[token.SessionID]
	if !found || token.RevokedAt != nil || !token.ExpiresAt.After(now) || session.RevokedAt != nil || !session.AbsoluteExpiresAt.After(now) {
		return nil, nil, domain.ErrRefreshInvalid
	}
	user := repository.users.user(session.UserID)
	if !user.Active() {
		return nil, nil, domain.ErrRefreshInvalid
	}
	if token.UsedAt != nil {
		repository.revokeSessionLocked(session.ID, now, "refresh_replay")
		return nil, nil, domain.ErrRefreshReplay
	}
	value := now
	token.UsedAt = &value
	repository.tokens[currentHash] = token
	repository.nextTokenID++
	replacement.ID = repository.nextTokenID
	replacement.SessionID = session.ID
	repository.tokens[replacement.TokenHash] = *replacement
	session.LastSeenAt = now
	repository.sessions[session.ID] = session
	return copySession(&session), copyToken(replacement), nil
}

func (repository *sessionRepositoryFake) ValidateAccessSession(_ context.Context, sessionID int64, now time.Time) (domain.Subject, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.lastValidatedAt = now
	subject, found := repository.subjects[sessionID]
	session, sessionFound := repository.sessions[sessionID]
	if !found || !sessionFound || session.RevokedAt != nil || !session.AbsoluteExpiresAt.After(now) {
		return domain.Subject{}, domain.SessionInvalid()
	}
	if repository.users != nil {
		user := repository.users.user(session.UserID)
		if !user.Active() {
			return domain.Subject{}, domain.SessionInvalid()
		}
		subject.Role = user.Role
	}
	return subject, nil
}

func (repository *sessionRepositoryFake) RevokeSession(ctx context.Context, sessionID int64, reason string, now time.Time) error {
	if _, found := database.TransactionFromContext(ctx); !found {
		return errors.New("revoke did not use runtime transaction")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.revokeSessionLocked(sessionID, now, reason)
	return nil
}

func (repository *sessionRepositoryFake) RevokeAllForUser(ctx context.Context, userID int64, reason string, now time.Time) error {
	if _, found := database.TransactionFromContext(ctx); !found {
		return errors.New("revoke-all did not use runtime transaction")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for id, session := range repository.sessions {
		if session.UserID == userID {
			repository.revokeSessionLocked(id, now, reason)
		}
	}
	return nil
}

func (repository *sessionRepositoryFake) revokeSessionLocked(sessionID int64, now time.Time, reason string) {
	session, found := repository.sessions[sessionID]
	if !found {
		return
	}
	if session.RevokedAt == nil {
		value := now
		session.RevokedAt = &value
		session.RevokeReason = reason
		repository.sessions[sessionID] = session
	}
	for hash, token := range repository.tokens {
		if token.SessionID == sessionID && token.RevokedAt == nil {
			value := now
			token.RevokedAt = &value
			repository.tokens[hash] = token
		}
	}
}

func (repository *sessionRepositoryFake) activeSessionCount(userID int64) int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	count := 0
	for _, session := range repository.sessions {
		if session.UserID == userID && session.RevokedAt == nil {
			count++
		}
	}
	return count
}

func copySession(value *domain.Session) *domain.Session {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyToken(value *domain.RefreshToken) *domain.RefreshToken {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type auditRepositoryFake struct {
	mu      sync.Mutex
	entries []domain.AuditEntry
}

func (repository *auditRepositoryFake) Create(ctx context.Context, entry domain.AuditEntry) error {
	if _, found := database.TransactionFromContext(ctx); !found {
		return errors.New("audit write did not use runtime transaction")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.entries = append(repository.entries, entry)
	return nil
}

var (
	_ domain.UserRepository    = (*userRepositoryFake)(nil)
	_ domain.SessionRepository = (*sessionRepositoryFake)(nil)
	_ domain.AuditRepository   = (*auditRepositoryFake)(nil)
	_ domain.PasswordHasher    = passwordHasherFake{}
	_ domain.TokenIssuer       = (*tokenIssuerFake)(nil)
	_ domain.VerificationStore = (*verificationStoreFake)(nil)
	_ domain.Mailer            = mailerFake{}
)
