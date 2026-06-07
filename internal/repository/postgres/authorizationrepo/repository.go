package authorizationrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/authorization"
	"github.com/StephenQiu30/hotkey-server/internal/platform/postgres"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateAuthorization(ctx context.Context, az authorization.Authorization) (authorization.Authorization, error) {
	const query = `
INSERT INTO authorizations (
    id, user_id, platform, platform_user_id, display_name, 
    access_token_enc, refresh_token_enc, status, 
    connected_at, last_checked_at, expires_at, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, user_id, platform, platform_user_id, display_name, 
          access_token_enc, refresh_token_enc, status, 
          connected_at, last_checked_at, expires_at, revoked_at, created_at, updated_at`

	row := postgres.GetQueryer(ctx, r.db).QueryRowContext(ctx, query,
		az.ID,
		az.UserID,
		az.Platform,
		az.PlatformUserID,
		az.DisplayName,
		az.AccessTokenEnc,
		az.RefreshTokenEnc,
		az.Status,
		az.ConnectedAt,
		az.LastCheckedAt,
		az.ExpiresAt,
		az.CreatedAt,
		az.UpdatedAt,
	)
	created, err := scanAuthorization(row)
	if err != nil {
		if isUniqueViolation(err) {
			return authorization.Authorization{}, authorization.ErrUniqueViolation
		}
		return authorization.Authorization{}, err
	}
	return created, nil
}

func (r *Repository) AuthorizationByID(ctx context.Context, id string) (authorization.Authorization, error) {
	const query = `
SELECT id, user_id, platform, platform_user_id, display_name, 
       access_token_enc, refresh_token_enc, status, 
       connected_at, last_checked_at, expires_at, revoked_at, created_at, updated_at
FROM authorizations
WHERE id = $1`
	az, err := scanAuthorization(postgres.GetQueryer(ctx, r.db).QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authorization.Authorization{}, authorization.ErrNotFound
		}
		return authorization.Authorization{}, err
	}
	return az, nil
}

func (r *Repository) AuthorizationsByUserID(ctx context.Context, userID string) ([]authorization.Authorization, error) {
	const query = `
SELECT id, user_id, platform, platform_user_id, display_name, 
       access_token_enc, refresh_token_enc, status, 
       connected_at, last_checked_at, expires_at, revoked_at, created_at, updated_at
FROM authorizations
WHERE user_id = $1
ORDER BY created_at DESC`

	rows, err := postgres.GetQueryer(ctx, r.db).QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []authorization.Authorization
	for rows.Next() {
		az, err := scanAuthorization(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, az)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) UpdateAuthorizationStatus(ctx context.Context, id string, status authorization.Status, now time.Time) error {
	var query string
	var args []any
	if status == authorization.StatusRevoked {
		query = `UPDATE authorizations SET status = $1, revoked_at = $2, updated_at = $2 WHERE id = $3`
		args = []any{status, now, id}
	} else {
		query = `UPDATE authorizations SET status = $1, updated_at = $2 WHERE id = $3`
		args = []any{status, now, id}
	}

	result, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return authorization.ErrNotFound
	}
	return nil
}

func (r *Repository) TouchAuthorization(ctx context.Context, id string, now time.Time) error {
	const query = `UPDATE authorizations SET last_checked_at = $1, updated_at = $1 WHERE id = $2`
	result, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, now, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return authorization.ErrNotFound
	}
	return nil
}

func (r *Repository) RevokeAllByUserID(ctx context.Context, userID string, now time.Time) error {
	const query = `
UPDATE authorizations 
SET status = $1, revoked_at = $2, updated_at = $2 
WHERE user_id = $3 AND status != $1`
	_, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, authorization.StatusRevoked, now, userID)
	return err
}

func (r *Repository) DeleteAuthorizationsByUserID(ctx context.Context, userID string) error {
	const query = `DELETE FROM authorizations WHERE user_id = $1`
	_, err := postgres.GetQueryer(ctx, r.db).ExecContext(ctx, query, userID)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAuthorization(s scanner) (authorization.Authorization, error) {
	var az authorization.Authorization
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	if err := s.Scan(
		&az.ID,
		&az.UserID,
		&az.Platform,
		&az.PlatformUserID,
		&az.DisplayName,
		&az.AccessTokenEnc,
		&az.RefreshTokenEnc,
		&az.Status,
		&az.ConnectedAt,
		&az.LastCheckedAt,
		&expiresAt,
		&revokedAt,
		&az.CreatedAt,
		&az.UpdatedAt,
	); err != nil {
		return authorization.Authorization{}, err
	}
	if expiresAt.Valid {
		az.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		az.RevokedAt = &revokedAt.Time
	}
	return az, nil
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
