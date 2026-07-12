package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/StephenQiu30/hotkey-server/internal/convert"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"gorm.io/gorm"
)

// UserRepo implements auth.UserRepository via GORM.
type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// IsUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *UserRepo) ExistsByEmail(ctx context.Context, email string) bool {
	u, err := r.GetByEmail(ctx, email)
	return err == nil && u != nil
}

func (r *UserRepo) Create(ctx context.Context, email, passwordHash, displayName string) (dto.User, error) {
	m := entity.User{
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		if IsUniqueViolation(err) {
			return dto.User{}, errors.New("email already registered")
		}
		return dto.User{}, err
	}
	return convert.UserEntityToDTO(m), nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*dto.User, error) {
	var m entity.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := convert.UserEntityToDTO(m)
	return &result, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*dto.User, error) {
	var m entity.User
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := convert.UserEntityToDTO(m)
	return &result, nil
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userID int64, newPasswordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash":       newPasswordHash,
		"password_changed_at": now,
		"updated_at":          now,
	}).Error
}

func (r *UserRepo) UpdateLastLogin(ctx context.Context, userID int64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
		"last_login_at": now,
		"updated_at":    now,
	}).Error
}

func (r *UserRepo) SetEmailVerified(ctx context.Context, userID int64, now time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
		"email_verified_at": now,
		"verification_status": "verified",
		"status":             "active",
		"updated_at":         now,
	}).Error
}

// txUserRepo wraps a *gorm.DB transaction to implement UserRepository.
type txUserRepo struct {
	tx *gorm.DB
}

func (r *txUserRepo) ExistsByEmail(ctx context.Context, email string) bool {
	u, err := r.GetByEmail(ctx, email)
	return err == nil && u != nil
}

func (r *txUserRepo) Create(ctx context.Context, email, passwordHash, displayName string) (dto.User, error) {
	m := entity.User{
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
	}
	if err := r.tx.WithContext(ctx).Create(&m).Error; err != nil {
		if IsUniqueViolation(err) {
			return dto.User{}, errors.New("email already registered")
		}
		return dto.User{}, err
	}
	return convert.UserEntityToDTO(m), nil
}

func (r *txUserRepo) GetByEmail(ctx context.Context, email string) (*dto.User, error) {
	var m entity.User
	if err := r.tx.WithContext(ctx).Where("email = ?", email).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := convert.UserEntityToDTO(m)
	return &result, nil
}

func (r *txUserRepo) GetByID(ctx context.Context, id int64) (*dto.User, error) {
	var m entity.User
	if err := r.tx.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := convert.UserEntityToDTO(m)
	return &result, nil
}

func (r *txUserRepo) UpdatePassword(ctx context.Context, userID int64, newPasswordHash string, now time.Time) error {
	return r.tx.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash":       newPasswordHash,
		"password_changed_at": now,
		"updated_at":          now,
	}).Error
}

func (r *txUserRepo) UpdateLastLogin(ctx context.Context, userID int64, now time.Time) error {
	return r.tx.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
		"last_login_at": now,
		"updated_at":    now,
	}).Error
}

func (r *txUserRepo) SetEmailVerified(ctx context.Context, userID int64, now time.Time) error {
	return r.tx.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
		"email_verified_at":   now,
		"verification_status": "verified",
		"status":              "active",
		"updated_at":          now,
	}).Error
}

// Transaction is a no-op for txUserRepo since it already runs inside a transaction.
// It calls fn directly; if fn returns an error, the outer transaction rolls back.
func (r *txUserRepo) Transaction(ctx context.Context, fn func(tx service.UserRepository) error) error {
	return fn(r)
}

// Transaction wraps the given function in a database transaction.
func (r *UserRepo) Transaction(ctx context.Context, fn func(tx service.UserRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&txUserRepo{tx: tx})
	})
}

// isUniqueConstraint checks if a GORM error is a PostgreSQL unique constraint error.
// Legacy: prefer IsUniqueViolation for direct pgx errors.
func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
