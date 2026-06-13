package database

import (
	"context"
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
)

// AuthRepo implements auth.Repository using PostgreSQL.
type AuthRepo struct {
	db *sql.DB
}

// NewAuthRepo creates a new Postgres-backed auth repository.
func NewAuthRepo(db *sql.DB) *AuthRepo {
	return &AuthRepo{db: db}
}

func (r *AuthRepo) ExistsByEmail(ctx context.Context, email string) bool {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", email,
	).Scan(&exists)
	return err == nil && exists
}

func (r *AuthRepo) Create(ctx context.Context, email, passwordHash, displayName string) (auth.User, error) {
	var u auth.User
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, password_hash, display_name, status, plan_type, created_at, updated_at`,
		email, passwordHash, displayName,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status, &u.PlanType, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (r *AuthRepo) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	var u auth.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status, &u.PlanType, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *AuthRepo) GetByID(ctx context.Context, id int64) (*auth.User, error) {
	var u auth.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status, &u.PlanType, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
