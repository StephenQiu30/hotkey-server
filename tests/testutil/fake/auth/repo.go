package fakeauth

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
)

// Repo is an in-memory fake implementing auth.Repository.
type Repo struct {
	Users  []auth.User
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

func (r *Repo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	r.nextID++
	now := time.Now()
	u := auth.User{
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

func (r *Repo) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	for i := range r.Users {
		if r.Users[i].Email == email {
			return &r.Users[i], nil
		}
	}
	return nil, nil
}

func (r *Repo) GetByID(_ context.Context, id int64) (*auth.User, error) {
	for i := range r.Users {
		if r.Users[i].ID == id {
			return &r.Users[i], nil
		}
	}
	return nil, nil
}
