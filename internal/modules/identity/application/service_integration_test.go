package application

import (
	"context"
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
