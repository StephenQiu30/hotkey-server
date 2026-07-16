package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var ErrBootstrapUnavailable = errors.New("bootstrap admin is unavailable after users exist")

const bootstrapAdminLock = "hotkey-identity-bootstrap-admin-v1"

type UserRepository struct {
	runtime *database.Runtime
}

var _ domain.UserRepository = (*UserRepository)(nil)

func NewUserRepository(runtime *database.Runtime) *UserRepository {
	return &UserRepository{runtime: runtime}
}

func (repository *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return nil, fmt.Errorf("normalize email: %w", err)
	}
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	return findUser(ctx, transactionSQL(ctx, repository.runtime), `
SELECT id, email, password_hash, display_name, role, status, last_login_at, created_at, updated_at, deleted_at
FROM users
WHERE lower(email) = lower($1) AND deleted_at IS NULL`, normalized)
}

func (repository *UserRepository) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: user ID must be positive", sharedrepository.ErrInvalidInput)
	}
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	return findUser(ctx, transactionSQL(ctx, repository.runtime), `
SELECT id, email, password_hash, display_name, role, status, last_login_at, created_at, updated_at, deleted_at
FROM users
WHERE id = $1 AND deleted_at IS NULL`, id)
}

// ListUsers returns every user record in stable identifier order. Soft-deleted
// records stay visible because restore administration needs that domain fact.
func (repository *UserRepository) ListUsers(ctx context.Context) ([]domain.User, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := transactionRows(ctx, repository.runtime).QueryContext(ctx, `
SELECT id, email, password_hash, display_name, role, status, last_login_at, created_at, updated_at, deleted_at
FROM users
ORDER BY id ASC`)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}
	return users, nil
}

// LockByID includes soft-deleted users so lifecycle restore operations can
// make their conflict and last-admin checks while holding the target row.
func (repository *UserRepository) LockByID(ctx context.Context, id int64) (*domain.User, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: user ID must be positive", sharedrepository.ErrInvalidInput)
	}
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	var user domain.User
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		var err error
		user, err = lockUserByID(ctx, transaction.SQL, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (repository *UserRepository) Create(ctx context.Context, user *domain.User) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if user == nil {
		return fmt.Errorf("%w: user is required", sharedrepository.ErrInvalidInput)
	}
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		return repository.createWithPreference(ctx, transaction, user)
	})
}

func (repository *UserRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string, now time.Time) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if id <= 0 || strings.TrimSpace(passwordHash) == "" || now.IsZero() {
		return fmt.Errorf("%w: user ID, password hash, and update time are required", sharedrepository.ErrInvalidInput)
	}
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		user, err := lockUserByID(ctx, transaction.SQL, id)
		if err != nil {
			return err
		}
		if !user.Active() {
			return inactiveUser()
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE users
SET password_hash = $1, updated_at = $2
WHERE id = $3`, passwordHash, now.UTC(), id); err != nil {
			return mapRepositoryError(err)
		}
		return nil
	})
}

func (repository *UserRepository) TouchLogin(ctx context.Context, id int64, now time.Time) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if id <= 0 || now.IsZero() {
		return fmt.Errorf("%w: user ID and login time are required", sharedrepository.ErrInvalidInput)
	}
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		user, err := lockUserByID(ctx, transaction.SQL, id)
		if err != nil {
			return err
		}
		if !user.Active() {
			return inactiveUser()
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE users
SET last_login_at = $1, updated_at = $1
WHERE id = $2`, now.UTC(), id); err != nil {
			return mapRepositoryError(err)
		}
		return nil
	})
}

// ChangeRole locks every active administrator before the target row. This
// stable ordering serializes concurrent lifecycle changes and preserves the
// required last-active-admin invariant without exposing transactions to the
// application layer.
func (repository *UserRepository) ChangeRole(ctx context.Context, id int64, role domain.Role, now time.Time) (*domain.User, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if id <= 0 || !role.Valid() || now.IsZero() {
		return nil, fmt.Errorf("%w: user ID, role, and update time are required", sharedrepository.ErrInvalidInput)
	}
	var changed domain.User
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		admins, err := lockActiveAdmins(ctx, transaction.SQL)
		if err != nil {
			return err
		}
		user, err := lockUserByID(ctx, transaction.SQL, id)
		if err != nil {
			return err
		}
		if user.DeletedAt != nil {
			return inactiveUser()
		}
		if removesLastActiveAdmin(user, len(admins), role, user.Status, false) {
			return domain.LastActiveAdmin()
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE users
SET role = $1, updated_at = $2
WHERE id = $3`, string(role), now.UTC(), id); err != nil {
			return mapRepositoryError(err)
		}
		changed, err = lockUserByID(ctx, transaction.SQL, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &changed, nil
}

// ChangeStatus takes the same active-admin lock as ChangeRole so a disable
// cannot race another lifecycle operation into removing the final admin.
func (repository *UserRepository) ChangeStatus(ctx context.Context, id int64, status domain.UserStatus, now time.Time) (*domain.User, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if id <= 0 || !status.Valid() || now.IsZero() {
		return nil, fmt.Errorf("%w: user ID, status, and update time are required", sharedrepository.ErrInvalidInput)
	}
	var changed domain.User
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		admins, err := lockActiveAdmins(ctx, transaction.SQL)
		if err != nil {
			return err
		}
		user, err := lockUserByID(ctx, transaction.SQL, id)
		if err != nil {
			return err
		}
		if user.DeletedAt != nil {
			return inactiveUser()
		}
		if removesLastActiveAdmin(user, len(admins), user.Role, status, false) {
			return domain.LastActiveAdmin()
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE users
SET status = $1, updated_at = $2
WHERE id = $3`, string(status), now.UTC(), id); err != nil {
			return mapRepositoryError(err)
		}
		changed, err = lockUserByID(ctx, transaction.SQL, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &changed, nil
}

func (repository *UserRepository) SoftDelete(ctx context.Context, id int64, now time.Time) (*domain.User, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if id <= 0 || now.IsZero() {
		return nil, fmt.Errorf("%w: user ID and deletion time are required", sharedrepository.ErrInvalidInput)
	}
	var deleted domain.User
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		admins, err := lockActiveAdmins(ctx, transaction.SQL)
		if err != nil {
			return err
		}
		user, err := lockUserByID(ctx, transaction.SQL, id)
		if err != nil {
			return err
		}
		if user.DeletedAt != nil {
			return fmt.Errorf("%w: user is already deleted", sharedrepository.ErrConflict)
		}
		if removesLastActiveAdmin(user, len(admins), user.Role, user.Status, true) {
			return domain.LastActiveAdmin()
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE users
SET deleted_at = $1, updated_at = $1
WHERE id = $2`, now.UTC(), id); err != nil {
			return mapRepositoryError(err)
		}
		deleted, err = lockUserByID(ctx, transaction.SQL, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &deleted, nil
}

func (repository *UserRepository) RestoreDisabled(ctx context.Context, id int64, now time.Time) (*domain.User, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if id <= 0 || now.IsZero() {
		return nil, fmt.Errorf("%w: user ID and restore time are required", sharedrepository.ErrInvalidInput)
	}
	var restored domain.User
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		user, err := lockUserByID(ctx, transaction.SQL, id)
		if err != nil {
			return err
		}
		if user.DeletedAt == nil {
			return fmt.Errorf("%w: user is not deleted", sharedrepository.ErrConflict)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE users
SET deleted_at = NULL, status = 'disabled', updated_at = $1
WHERE id = $2`, now.UTC(), id); err != nil {
			return mapRepositoryError(err)
		}
		restored, err = lockUserByID(ctx, transaction.SQL, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &restored, nil
}

// BootstrapAdmin creates exactly one initial administrator while a transaction
// advisory lock serializes concurrent local command invocations. It never
// accepts a caller-provided role or status.
func (repository *UserRepository) BootstrapAdmin(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return nil, fmt.Errorf("normalize bootstrap email: %w", err)
	}
	if strings.TrimSpace(passwordHash) == "" {
		return nil, fmt.Errorf("%w: password hash is required", sharedrepository.ErrInvalidInput)
	}

	admin := &domain.User{
		Email:        normalized,
		PasswordHash: passwordHash,
		DisplayName:  "Administrator",
		Role:         domain.RoleAdmin,
		Status:       domain.UserStatusActive,
	}
	err = useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, bootstrapAdminLock); err != nil {
			return mapRepositoryError(err)
		}
		var userCount int
		if err := transaction.SQL.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE deleted_at IS NULL`).Scan(&userCount); err != nil {
			return mapRepositoryError(err)
		}
		if userCount != 0 {
			return ErrBootstrapUnavailable
		}
		return repository.createWithPreference(ctx, transaction, admin)
	})
	if err != nil {
		return nil, err
	}
	return admin, nil
}

// LockActiveAdmins is deliberately transaction-scoped. The application layer
// uses this together with the target user lock before changing any admin's
// role, status, or deletion state.
func (repository *UserRepository) LockActiveAdmins(ctx context.Context) ([]domain.User, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	admins := make([]domain.User, 0)
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		var err error
		admins, err = lockActiveAdmins(ctx, transaction.SQL)
		return err
	})
	if err != nil {
		return nil, err
	}
	return admins, nil
}

type rowLocker interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type rowsLocker interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func lockUserByID(ctx context.Context, queryer rowLocker, id int64) (domain.User, error) {
	return scanUser(queryer.QueryRowContext(ctx, `
SELECT id, email, password_hash, display_name, role, status, last_login_at, created_at, updated_at, deleted_at
FROM users
WHERE id = $1
FOR UPDATE`, id))
}

func lockActiveAdmins(ctx context.Context, queryer rowsLocker) ([]domain.User, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT id, email, password_hash, display_name, role, status, last_login_at, created_at, updated_at, deleted_at
FROM users
WHERE role = 'admin' AND status = 'active' AND deleted_at IS NULL
ORDER BY id
FOR UPDATE`)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()
	admins := make([]domain.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		admins = append(admins, user)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}
	return admins, nil
}

func removesLastActiveAdmin(user domain.User, activeAdminCount int, role domain.Role, status domain.UserStatus, deleted bool) bool {
	return user.Active() && user.Role == domain.RoleAdmin && activeAdminCount == 1 && (deleted || role != domain.RoleAdmin || status != domain.UserStatusActive)
}

func inactiveUser() error {
	return fmt.Errorf("%w: user is not active", sharedrepository.ErrNotFound)
}

func (repository *UserRepository) createWithPreference(ctx context.Context, transaction database.Transaction, user *domain.User) error {
	if transaction.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	normalized, err := domain.NormalizeEmail(user.Email)
	if err != nil {
		return fmt.Errorf("normalize email: %w", err)
	}
	if strings.TrimSpace(user.PasswordHash) == "" || strings.TrimSpace(user.DisplayName) == "" || !user.Role.Valid() || !user.Status.Valid() {
		return fmt.Errorf("%w: user fields are invalid", sharedrepository.ErrInvalidInput)
	}

	var record userRecord
	err = transaction.SQL.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, display_name, role, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, password_hash, display_name, role, status, last_login_at, created_at, updated_at, deleted_at`,
		normalized, user.PasswordHash, user.DisplayName, string(user.Role), string(user.Status),
	).Scan(&record.ID, &record.Email, &record.PasswordHash, &record.DisplayName, &record.Role, &record.Status, &record.LastLoginAt, &record.CreatedAt, &record.UpdatedAt, &record.DeletedAt)
	if err != nil {
		return mapRepositoryError(err)
	}
	if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO user_preferences (user_id) VALUES ($1)`, record.ID); err != nil {
		return mapRepositoryError(err)
	}
	*user = record.domainUser()
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func findUser(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, arguments ...any) (*domain.User, error) {
	user, err := scanUser(queryer.QueryRowContext(ctx, query, arguments...))
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func scanUser(scanner rowScanner) (domain.User, error) {
	var record userRecord
	if err := scanner.Scan(&record.ID, &record.Email, &record.PasswordHash, &record.DisplayName, &record.Role, &record.Status, &record.LastLoginAt, &record.CreatedAt, &record.UpdatedAt, &record.DeletedAt); err != nil {
		return domain.User{}, mapRepositoryError(err)
	}
	return record.domainUser(), nil
}

func mapRepositoryError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return sharedrepository.ErrNotFound
	}
	return sharedrepository.MapError(err)
}
