package gormimpl

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
)

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

func (r *UserRepo) Create(ctx context.Context, email, passwordHash, displayName string) (model.User, error) {
	m := User{
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return model.User{}, err
	}
	return ToUser(m), nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var m User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := ToUser(m)
	return &result, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var m User
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := ToUser(m)
	return &result, nil
}
