package auth

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is an in-memory implementation of Repository for testing.
type fakeRepo struct {
	users []User
}

func (r *fakeRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}

func (r *fakeRepo) Create(_ context.Context, email, passwordHash, displayName string) (User, error) {
	user := User{
		ID:           int64(len(r.users) + 1),
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		Status:       "active",
		PlanType:     "free",
	}
	r.users = append(r.users, user)
	return user, nil
}

func (r *fakeRepo) GetByEmail(_ context.Context, email string) (*User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	repo := &fakeRepo{
		users: []User{{Email: "user@example.com", PasswordHash: "hash"}},
	}
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("expected ErrEmailExists, got %v", err)
	}
}

func TestRegisterRejectsEmptyEmail(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRegisterRejectsShortPassword(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "user@example.com",
		Password:    "short",
		DisplayName: "User",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRegisterSuccess(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	user, err := svc.Register(context.Background(), RegisterInput{
		Email:       "new@example.com",
		Password:    "Passw0rd!",
		DisplayName: "New User",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Email != "new@example.com" {
		t.Fatalf("expected email new@example.com, got %s", user.Email)
	}
	if user.PasswordHash == "Passw0rd!" {
		t.Fatal("password should be hashed, not plaintext")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	// Register first
	_, _ = svc.Register(context.Background(), RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	// Try login with wrong password
	_, err := svc.Login(context.Background(), LoginInput{
		Email:    "user@example.com",
		Password: "WrongPass!",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	_, err := svc.Login(context.Background(), LoginInput{
		Email:    "nobody@example.com",
		Password: "Passw0rd!",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginSuccess(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	_, _ = svc.Register(context.Background(), RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	user, err := svc.Login(context.Background(), LoginInput{
		Email:    "user@example.com",
		Password: "Passw0rd!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %s", user.Email)
	}
}
