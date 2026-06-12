package auth

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is a test double that satisfies Repository.
type fakeRepo struct {
	existingEmail string
	users         []User
	passwordHashes []string
}

func (r *fakeRepo) ExistsByEmail(_ context.Context, email string) bool {
	return email == r.existingEmail
}

func (r *fakeRepo) Create(_ context.Context, email, passwordHash, displayName string) (User, error) {
	u := User{
		ID:          int64(len(r.users) + 1),
		Email:       email,
		DisplayName: displayName,
		Status:      "active",
		PlanType:    "free",
	}
	r.users = append(r.users, u)
	r.passwordHashes = append(r.passwordHashes, passwordHash)
	return u, nil
}

func (r *fakeRepo) FindByEmail(_ context.Context, email string) (User, string, error) {
	for i, u := range r.users {
		if u.Email == email {
			return u, r.passwordHashes[i], nil
		}
	}
	return User{}, "", errors.New("not found")
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	repo := &fakeRepo{existingEmail: "user@example.com"}
	svc := NewService(repo, "test-secret")
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("expected ErrEmailExists, got %v", err)
	}
}

func TestRegisterCreatesUser(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, "test-secret")
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
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	repo := &fakeRepo{
		users:          []User{{ID: 1, Email: "user@example.com", Status: "active"}},
		passwordHashes: []string{"$2a$10$invalidhashthatdoesnotmatchanything"},
	}
	svc := NewService(repo, "test-secret")
	_, err := svc.Login(context.Background(), LoginInput{
		Email:    "user@example.com",
		Password: "WrongPass!",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
