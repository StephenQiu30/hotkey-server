package userrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	"github.com/StephenQiu30/hotkey-server/internal/platform/postgres"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, account user.User) (user.User, error) {
	const query = `
INSERT INTO users (id, email, password_hash, role, status, timezone, daily_send_at, wechat_open_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10)
RETURNING id, email, password_hash, role, status, timezone, daily_send_at, COALESCE(wechat_open_id, ''), created_at, updated_at`
	row := postgres.GetQueryer(ctx, r.db).QueryRowContext(ctx, query,
		account.ID,
		account.Email,
		account.PasswordHash,
		account.Role,
		account.Status,
		account.Timezone,
		account.DailySendAt,
		account.WeChatOpenID,
		account.CreatedAt,
		account.UpdatedAt,
	)
	created, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return user.User{}, serviceauth.ErrEmailAlreadyExists
		}
		return user.User{}, err
	}
	return created, nil
}

func (r *Repository) UserByEmail(ctx context.Context, email string) (user.User, error) {
	const query = `
SELECT id, email, password_hash, role, status, timezone, daily_send_at, COALESCE(wechat_open_id, ''), created_at, updated_at
FROM users
WHERE email = $1`
	return scanUser(postgres.GetQueryer(ctx, r.db).QueryRowContext(ctx, query, email))
}

func (r *Repository) UserByID(ctx context.Context, id string) (user.User, error) {
	const query = `
SELECT id, email, password_hash, role, status, timezone, daily_send_at, COALESCE(wechat_open_id, ''), created_at, updated_at
FROM users
WHERE id = $1`
	return scanUser(postgres.GetQueryer(ctx, r.db).QueryRowContext(ctx, query, id))
}

func (r *Repository) CreateRefreshToken(ctx context.Context, token serviceauth.RefreshToken) error {
	const query = `
INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.RevokedAt, token.CreatedAt)
	return err
}

func (r *Repository) RefreshTokenByHash(ctx context.Context, tokenHash string) (serviceauth.RefreshToken, error) {
	const query = `
SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
FROM refresh_tokens
WHERE token_hash = $1`
	var token serviceauth.RefreshToken
	var revokedAt sql.NullTime
	if err := postgres.GetQueryer(ctx, r.db).QueryRowContext(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&revokedAt,
		&token.CreatedAt,
	); err != nil {
		return serviceauth.RefreshToken{}, err
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	return token, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string, revokedAt time.Time) error {
	const query = `UPDATE refresh_tokens SET revoked_at = $1 WHERE token_hash = $2`
	result, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, revokedAt, tokenHash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(row userScanner) (user.User, error) {
	var account user.User
	if err := row.Scan(
		&account.ID,
		&account.Email,
		&account.PasswordHash,
		&account.Role,
		&account.Status,
		&account.Timezone,
		&account.DailySendAt,
		&account.WeChatOpenID,
		&account.CreatedAt,
		&account.UpdatedAt,
	); err != nil {
		return user.User{}, err
	}
	return account, nil
}

func (r *Repository) UpdateUser(ctx context.Context, account user.User) error {
	const query = `
UPDATE users SET email = $2, password_hash = $3, role = $4, status = $5, timezone = $6, daily_send_at = $7, wechat_open_id = NULLIF($8, ''), updated_at = $9
WHERE id = $1`
	result, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query,
		account.ID,
		account.Email,
		account.PasswordHash,
		account.Role,
		account.Status,
		account.Timezone,
		account.DailySendAt,
		account.WeChatOpenID,
		account.UpdatedAt,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) RevokeAllTokensForUser(ctx context.Context, userID string, revokedAt time.Time) error {
	const query = `UPDATE refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL`
	_, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, revokedAt, userID)
	return err
}

func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	const query = `DELETE FROM users WHERE id = $1`
	result, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteRefreshTokensByUserID(ctx context.Context, userID string) error {
	const query = `DELETE FROM refresh_tokens WHERE user_id = $1`
	_, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, userID)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
