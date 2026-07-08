package gormimpl

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)

// UserRepo implements auth.Repository via GORM.
type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) ExistsByEmail(ctx context.Context, email string) bool {
	u, err := r.GetByEmail(ctx, email)
	return err == nil && u != nil
}

func (r *UserRepo) Create(ctx context.Context, email, passwordHash, displayName string) (auth.User, error) {
	m := entity.User{
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return auth.User{}, err
	}
	return toAuthUser(m), nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	var m entity.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := toAuthUser(m)
	return &result, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*auth.User, error) {
	var m entity.User
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := toAuthUser(m)
	return &result, nil
}

func toAuthUser(m entity.User) auth.User {
	return auth.User{
		ID:           m.ID,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		DisplayName:  m.DisplayName,
		Status:       m.Status,
		PlanType:     m.PlanType,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
