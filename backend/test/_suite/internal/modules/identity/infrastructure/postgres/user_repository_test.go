package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
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

func TestUserRepositoryListCursorIsSignedFilterBoundExpiringAndSnapshotStable(t *testing.T) {
	runtime := newIdentityRuntime(t)
	codec, err := pagination.NewCodec(strings.Repeat("identity-user-list-secret-", 2), time.Minute)
	if err != nil {
		t.Fatalf("pagination.NewCodec(): %v", err)
	}
	repository := NewUserRepositoryWithCursorCodec(runtime, codec)

	first := createIdentityUser(t, repository, "list-first")
	second := createIdentityUser(t, repository, "list-second")
	third := createIdentityUser(t, repository, "list-third")
	if _, err := repository.ChangeRole(context.Background(), third.ID, domain.RoleEditor, time.Now().UTC()); err != nil {
		t.Fatalf("ChangeRole(): %v", err)
	}
	deletedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	if _, err := repository.SoftDelete(context.Background(), second.ID, deletedAt); err != nil {
		t.Fatalf("SoftDelete(): %v", err)
	}

	firstPage, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 2})
	if err != nil || len(firstPage.Items) != 2 || firstPage.NextCursor == "" {
		t.Fatalf("ListUsers(first) = %#v, %v", firstPage, err)
	}
	if firstPage.Items[0].ID != first.ID || firstPage.Items[1].ID != second.ID || strings.Count(firstPage.NextCursor, ".") != 1 {
		t.Fatalf("ListUsers(first) users/cursor = %#v/%q", firstPage.Items, firstPage.NextCursor)
	}
	if firstPage.Items[1].DeletedAt == nil || !firstPage.Items[1].DeletedAt.Equal(deletedAt) {
		t.Fatalf("ListUsers(first) deleted user = %#v", firstPage.Items[1])
	}
	concurrent := createIdentityUser(t, repository, "list-concurrent")
	secondPage, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 2, Cursor: firstPage.NextCursor})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != third.ID || secondPage.NextCursor != "" {
		t.Fatalf("ListUsers(second) = %#v, %v; concurrent=%d", secondPage, err, concurrent.ID)
	}
	fresh, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 10})
	if err != nil || len(fresh.Items) != 4 || fresh.Items[3].ID != concurrent.ID {
		t.Fatalf("ListUsers(fresh) = %#v, %v", fresh, err)
	}

	tampered := firstPage.NextCursor[:len(firstPage.NextCursor)-1] + "A"
	if strings.HasSuffix(firstPage.NextCursor, "A") {
		tampered = firstPage.NextCursor[:len(firstPage.NextCursor)-1] + "B"
	}
	if _, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 2, Cursor: tampered}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("tampered cursor error = %v", err)
	}
	editor := domain.RoleEditor
	for name, query := range map[string]domain.UserListQuery{
		"search": {Limit: 2, Cursor: firstPage.NextCursor, Search: "list-first"},
		"role":   {Limit: 2, Cursor: firstPage.NextCursor, Role: &editor},
		"status": {Limit: 2, Cursor: firstPage.NextCursor, Status: domain.UserListStatusDeleted},
	} {
		if _, err := repository.ListUsers(context.Background(), query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("cross-%s cursor error = %v", name, err)
		}
	}
	searchPage, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 10, Search: "LIST-FIRST"})
	if err != nil || len(searchPage.Items) != 1 || searchPage.Items[0].ID != first.ID {
		t.Fatalf("search page = %#v, %v", searchPage, err)
	}
	rolePage, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 10, Role: &editor})
	if err != nil || len(rolePage.Items) != 1 || rolePage.Items[0].ID != third.ID {
		t.Fatalf("role page = %#v, %v", rolePage, err)
	}
	deletedPage, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 10, Status: domain.UserListStatusDeleted})
	if err != nil || len(deletedPage.Items) != 1 || deletedPage.Items[0].ID != second.ID {
		t.Fatalf("deleted page = %#v, %v", deletedPage, err)
	}
	if _, err := repository.ListUsers(context.Background(), domain.UserListQuery{Limit: 201}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("oversized page error = %v", err)
	}

	shortCodec, err := pagination.NewCodec(strings.Repeat("identity-user-short-", 2), time.Millisecond)
	if err != nil {
		t.Fatalf("pagination.NewCodec(short): %v", err)
	}
	shortRepository := NewUserRepositoryWithCursorCodec(runtime, shortCodec)
	shortPage, err := shortRepository.ListUsers(context.Background(), domain.UserListQuery{Limit: 1})
	if err != nil || shortPage.NextCursor == "" {
		t.Fatalf("short first page = %#v, %v", shortPage, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := shortRepository.ListUsers(context.Background(), domain.UserListQuery{Limit: 1, Cursor: shortPage.NextCursor}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired cursor error = %v", err)
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

func TestUserRepositoryLocksActiveAdminsForLifecycleChecks(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	admin := createIdentityAdmin(t, runtime, repository, "admin-lock")

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
	admin := createIdentityAdmin(t, runtime, repository, "last-admin")
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

func TestUserRepositorySerializesConcurrentAdminDemotions(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewUserRepository(runtime)
	first := createIdentityAdmin(t, runtime, repository, "concurrent-admin-first")
	second := createIdentityAdmin(t, runtime, repository, "concurrent-admin-second")
	now := time.Now().UTC().Truncate(time.Microsecond)

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, id := range []int64{first.ID, second.ID} {
		id := id
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := repository.ChangeRole(context.Background(), id, domain.RoleViewer, now)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	succeeded := 0
	protected := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		var appError *sharederrors.AppError
		if errors.As(err, &appError) && appError.Code == sharederrors.CodeLastActiveAdmin {
			protected++
			continue
		}
		t.Fatalf("concurrent ChangeRole() error = %v, want success or CodeLastActiveAdmin", err)
	}
	if succeeded != 1 || protected != 1 {
		t.Fatalf("concurrent demotions = %d succeeded, %d protected; want 1 and 1", succeeded, protected)
	}

	var activeAdmins int
	if err := runtime.SQL.QueryRow(`
SELECT count(*)
FROM users
WHERE role = 'admin' AND status = 'active' AND deleted_at IS NULL`).Scan(&activeAdmins); err != nil {
		t.Fatalf("count active admins: %v", err)
	}
	if activeAdmins != 1 {
		t.Fatalf("active admins after concurrent demotions = %d, want 1", activeAdmins)
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
