package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
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

func TestUserRepositoryListsUsersByIDAndIncludesSoftDeletedFacts(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)

	first := createIdentityUser(t, repository, "list-first")
	second := createIdentityUser(t, repository, "list-second")
	third := createIdentityUser(t, repository, "list-third")
	deletedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	if _, err := repository.SoftDelete(context.Background(), second.ID, deletedAt); err != nil {
		t.Fatalf("SoftDelete(): %v", err)
	}

	users, err := repository.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers(): %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("ListUsers() returned %d users, want 3", len(users))
	}
	if users[0].ID != first.ID || users[1].ID != second.ID || users[2].ID != third.ID {
		t.Fatalf("ListUsers() IDs = [%d %d %d], want [%d %d %d] in ascending ID order", users[0].ID, users[1].ID, users[2].ID, first.ID, second.ID, third.ID)
	}
	if users[1].DeletedAt == nil || !users[1].DeletedAt.Equal(deletedAt) {
		t.Fatalf("ListUsers() deleted user = %#v, want persisted deleted_at %s", users[1], deletedAt)
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

func TestUserRepositoryUpdatesPasswordAndLastLoginWithinCallerTransaction(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	user := createIdentityUser(t, repository, "credentials")
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		if err := repository.UpdatePassword(ctx, user.ID, "new-bcrypt-hash", now); err != nil {
			return err
		}
		return repository.TouchLogin(ctx, user.ID, now.Add(time.Minute))
	}); err != nil {
		t.Fatalf("credential updates inside Runtime.WithinTransaction: %v", err)
	}

	var passwordHash string
	var lastLoginAt time.Time
	if err := runtime.SQL.QueryRow(`SELECT password_hash, last_login_at FROM users WHERE id = $1`, user.ID).Scan(&passwordHash, &lastLoginAt); err != nil {
		t.Fatalf("read updated credentials: %v", err)
	}
	if passwordHash != "new-bcrypt-hash" || !lastLoginAt.UTC().Equal(now.Add(time.Minute)) {
		t.Fatalf("credentials = password %q login %s, want updated password and login %s", passwordHash, lastLoginAt.UTC(), now.Add(time.Minute))
	}
}

func TestUserRepositoryChangesRoleAndStatus(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	user := createIdentityUser(t, repository, "lifecycle-updates")
	now := time.Now().UTC().Truncate(time.Microsecond)

	changedRole, err := repository.ChangeRole(context.Background(), user.ID, domain.RoleEditor, now)
	if err != nil {
		t.Fatalf("ChangeRole(): %v", err)
	}
	if changedRole.Role != domain.RoleEditor || changedRole.Status != domain.UserStatusActive {
		t.Fatalf("ChangeRole() = %#v, want active editor", changedRole)
	}
	changedStatus, err := repository.ChangeStatus(context.Background(), user.ID, domain.UserStatusDisabled, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ChangeStatus(): %v", err)
	}
	if changedStatus.Role != domain.RoleEditor || changedStatus.Status != domain.UserStatusDisabled {
		t.Fatalf("ChangeStatus() = %#v, want disabled editor", changedStatus)
	}
}

func TestUserRepositoryPreventsRemovingLastActiveAdmin(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	admin, err := repository.BootstrapAdmin(context.Background(), "last-admin@example.test", "bcrypt-hash")
	if err != nil {
		t.Fatalf("BootstrapAdmin(): %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{name: "role", run: func() error {
			_, err := repository.ChangeRole(context.Background(), admin.ID, domain.RoleViewer, now)
			return err
		}},
		{name: "status", run: func() error {
			_, err := repository.ChangeStatus(context.Background(), admin.ID, domain.UserStatusDisabled, now)
			return err
		}},
		{name: "delete", run: func() error { _, err := repository.SoftDelete(context.Background(), admin.ID, now); return err }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.run()
			var appError *sharederrors.AppError
			if !errors.As(err, &appError) || appError.Code != sharederrors.CodeLastActiveAdmin {
				t.Fatalf("last-admin %s error = %v, want CodeLastActiveAdmin", operation.name, err)
			}
			locked, err := repository.LockByID(context.Background(), admin.ID)
			if err != nil {
				t.Fatalf("LockByID(): %v", err)
			}
			if locked.Role != domain.RoleAdmin || locked.Status != domain.UserStatusActive || locked.DeletedAt != nil {
				t.Fatalf("last admin after %s = %#v, want unchanged active admin", operation.name, locked)
			}
		})
	}
}

func TestUserRepositorySoftDeletesAndRestoresDisabledUser(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	user := createIdentityUser(t, repository, "restore")
	now := time.Now().UTC().Truncate(time.Microsecond)

	deleted, err := repository.SoftDelete(context.Background(), user.ID, now)
	if err != nil {
		t.Fatalf("SoftDelete(): %v", err)
	}
	if deleted.DeletedAt == nil {
		t.Fatalf("SoftDelete() = %#v, want deleted user", deleted)
	}
	if _, err := repository.FindByEmail(context.Background(), user.Email); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("FindByEmail() after delete error = %v, want not found", err)
	}

	restored, err := repository.RestoreDisabled(context.Background(), user.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RestoreDisabled(): %v", err)
	}
	if restored.DeletedAt != nil || restored.Status != domain.UserStatusDisabled {
		t.Fatalf("RestoreDisabled() = %#v, want non-deleted disabled user", restored)
	}
	found, err := repository.FindByEmail(context.Background(), user.Email)
	if err != nil {
		t.Fatalf("FindByEmail() after restore: %v", err)
	}
	if found.ID != user.ID || found.Status != domain.UserStatusDisabled {
		t.Fatalf("restored user = %#v, want disabled original user", found)
	}
}

func TestUserRepositoryRestoreConflictingActiveEmailLeavesDeletedUserUnchanged(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	original := createIdentityUser(t, repository, "restore-conflict")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := repository.SoftDelete(context.Background(), original.ID, now); err != nil {
		t.Fatalf("SoftDelete(): %v", err)
	}
	replacement := &domain.User{
		Email:        original.Email,
		PasswordHash: "replacement-bcrypt-hash",
		DisplayName:  "Replacement User",
		Role:         domain.RoleViewer,
		Status:       domain.UserStatusActive,
	}
	if err := repository.Create(context.Background(), replacement); err != nil {
		t.Fatalf("Create replacement user: %v", err)
	}

	if _, err := repository.RestoreDisabled(context.Background(), original.ID, now.Add(time.Minute)); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("RestoreDisabled() error = %v, want repository conflict", err)
	}
	lockedOriginal, err := repository.LockByID(context.Background(), original.ID)
	if err != nil {
		t.Fatalf("LockByID(original): %v", err)
	}
	if lockedOriginal.DeletedAt == nil || lockedOriginal.Role != domain.RoleViewer || lockedOriginal.Status != domain.UserStatusActive {
		t.Fatalf("original after restore conflict = %#v, want unchanged deleted lifecycle state", lockedOriginal)
	}
	foundReplacement, err := repository.FindByEmail(context.Background(), original.Email)
	if err != nil {
		t.Fatalf("FindByEmail(replacement): %v", err)
	}
	if foundReplacement.ID != replacement.ID || foundReplacement.Status != domain.UserStatusActive {
		t.Fatalf("replacement after restore conflict = %#v, want unchanged active replacement", foundReplacement)
	}
}
