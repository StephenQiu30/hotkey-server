package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func TestUserRepositoryCreatesPreferenceAndEnforcesNormalizedEmailUniqueness(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)

	user := &domain.User{
		Email:        "  Admin@Example.Test ",
		PasswordHash: "bcrypt-hash",
		DisplayName:  "Admin",
		Role:         domain.RoleViewer,
		Status:       domain.UserStatusActive,
	}
	if err := repository.Create(context.Background(), user); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if user.ID <= 0 || user.Email != "admin@example.test" {
		t.Fatalf("created user = %#v, want persisted normalized user", user)
	}

	var preferenceCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM user_preferences WHERE user_id = $1`, user.ID).Scan(&preferenceCount); err != nil {
		t.Fatalf("count user preferences: %v", err)
	}
	if preferenceCount != 1 {
		t.Fatalf("user preferences = %d, want 1", preferenceCount)
	}

	found, err := repository.FindByEmail(context.Background(), "ADMIN@example.test")
	if err != nil {
		t.Fatalf("FindByEmail(): %v", err)
	}
	if found.ID != user.ID || found.Email != "admin@example.test" {
		t.Fatalf("FindByEmail() = %#v, want created user", found)
	}

	duplicate := *user
	duplicate.ID = 0
	duplicate.Email = "admin@example.test"
	if err := repository.Create(context.Background(), &duplicate); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("duplicate Create() error = %v, want repository conflict", err)
	}
}

func TestUserRepositoryReusesAnExistingRuntimeTransaction(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	user := &domain.User{
		Email:        "transaction@example.test",
		PasswordHash: "bcrypt-hash",
		DisplayName:  "Transaction User",
		Role:         domain.RoleViewer,
		Status:       domain.UserStatusActive,
	}
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		return repository.Create(ctx, user)
	}); err != nil {
		t.Fatalf("Create() inside Runtime.WithinTransaction: %v", err)
	}
	if user.ID <= 0 {
		t.Fatalf("created user ID = %d, want persisted user", user.ID)
	}
}

func TestUserRepositoryBootstrapAdminUsesTransactionLockAndRejectsNonemptyUsers(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)

	admin, err := repository.BootstrapAdmin(context.Background(), "first-admin@example.test", "bcrypt-hash")
	if err != nil {
		t.Fatalf("BootstrapAdmin(): %v", err)
	}
	if admin.Role != domain.RoleAdmin || admin.Status != domain.UserStatusActive {
		t.Fatalf("bootstrap user = %#v, want active admin", admin)
	}
	if _, err := repository.BootstrapAdmin(context.Background(), "second-admin@example.test", "bcrypt-hash"); !errors.Is(err, ErrBootstrapUnavailable) {
		t.Fatalf("second BootstrapAdmin() error = %v, want ErrBootstrapUnavailable", err)
	}
}

func TestUserRepositoryLocksActiveAdminsForLifecycleChecks(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	admin, err := repository.BootstrapAdmin(context.Background(), "admin-lock@example.test", "bcrypt-hash")
	if err != nil {
		t.Fatalf("BootstrapAdmin(): %v", err)
	}

	var locked []domain.User
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		var err error
		locked, err = repository.LockActiveAdmins(ctx)
		return err
	}); err != nil {
		t.Fatalf("LockActiveAdmins(): %v", err)
	}
	if len(locked) != 1 || locked[0].ID != admin.ID {
		t.Fatalf("locked admins = %#v, want bootstrap admin", locked)
	}
}

func TestUserRepositoryLocksTargetIncludingSoftDeletedLifecycleUser(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	user := createIdentityUser(t, repository, "lock-target")
	if _, err := runtime.SQL.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var locked *domain.User
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		var err error
		locked, err = repository.LockByID(ctx, user.ID)
		return err
	}); err != nil {
		t.Fatalf("LockByID(): %v", err)
	}
	if locked == nil || locked.ID != user.ID || locked.DeletedAt == nil {
		t.Fatalf("locked user = %#v, want soft-deleted target", locked)
	}
}
