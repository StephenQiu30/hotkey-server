package database

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"gorm.io/gorm"
)

// AuthRepo implements auth.Repository using PostgreSQL via GORM.
type AuthRepo struct {
	db *gorm.DB
}

// NewAuthRepo creates a new Postgres-backed auth repository.
func NewAuthRepo(db *gorm.DB) *AuthRepo {
	return &AuthRepo{db: db}
}

func (r *AuthRepo) ExistsByEmail(ctx context.Context, email string) bool {
	var exists bool
	err := r.db.WithContext(ctx).Raw(
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email,
	).Scan(&exists).Error
	return err == nil && exists
}

func (r *AuthRepo) Create(ctx context.Context, email, passwordHash, displayName string) (auth.User, error) {
	var u auth.User
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES (?, ?, ?)
		 RETURNING id, email, password_hash, display_name, status, plan_type, created_at, updated_at`,
		email, passwordHash, displayName,
	).Scan(&u).Error
	return u, err
}

func (r *AuthRepo) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	var u auth.User
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
		 FROM users WHERE email = ?`, email,
	).Scan(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && u.ID == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *AuthRepo) GetByID(ctx context.Context, id int64) (*auth.User, error) {
	var u auth.User
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
		 FROM users WHERE id = ?`, id,
	).Scan(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && u.ID == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
