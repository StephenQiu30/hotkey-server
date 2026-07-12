package fakeauth

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// Repo is an in-memory fake implementing service.AuthRepository.
type Repo struct {
	Users  []dto.User
	nextID int64
}

func (r *Repo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.Users {
		if u.Email == email {
			return true
		}
	}
	return false
}

func (r *Repo) Create(_ context.Context, email, passwordHash, displayName string) (dto.User, error) {
	r.nextID++
	now := time.Now()
	u := dto.User{
		ID:           r.nextID,
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	r.Users = append(r.Users, u)
	return u, nil
}

func (r *Repo) GetByEmail(_ context.Context, email string) (*dto.User, error) {
	for i := range r.Users {
		if r.Users[i].Email == email {
			return &r.Users[i], nil
		}
	}
	return nil, nil
}

func (r *Repo) GetByID(_ context.Context, id int64) (*dto.User, error) {
	for i := range r.Users {
		if r.Users[i].ID == id {
			return &r.Users[i], nil
		}
	}
	return nil, nil
}

func (r *Repo) UpdatePassword(_ context.Context, userID int64, newPasswordHash string, now time.Time) error {
	for i := range r.Users {
		if r.Users[i].ID == userID {
			r.Users[i].PasswordHash = newPasswordHash
			r.Users[i].PasswordChangedAt = &now
			return nil
		}
	}
	return nil
}

func (r *Repo) UpdateLastLogin(_ context.Context, userID int64, now time.Time) error {
	for i := range r.Users {
		if r.Users[i].ID == userID {
			r.Users[i].LastLoginAt = &now
			return nil
		}
	}
	return nil
}

func (r *Repo) SetEmailVerified(_ context.Context, userID int64, now time.Time) error {
	return nil
}

func (r *Repo) Transaction(_ context.Context, fn func(tx service.UserRepository) error) error {
	return fn(r)
}
