package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/auth"
)

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	repo := &fakeauth.Repo{
		Users: []dto.User{{Email: "user@example.com", PasswordHash: "hash"}},
	}
	svc := service.NewAuthService(repo)
	_, err := svc.Register(context.Background(), dto.RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	if !errors.Is(err, service.AuthErrEmailExists) {
		t.Fatalf("expected ErrEmailExists, got %v", err)
	}
}

func TestRegisterRejectsEmptyEmail(t *testing.T) {
	repo := &fakeauth.Repo{}
	svc := service.NewAuthService(repo)
	_, err := svc.Register(context.Background(), dto.RegisterInput{
		Email:       "",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	if !errors.Is(err, service.AuthErrInvalidInput) {
		t.Fatalf("expected AuthErrInvalidInput, got %v", err)
	}
}

func TestRegisterRejectsShortPassword(t *testing.T) {
	repo := &fakeauth.Repo{}
	svc := service.NewAuthService(repo)
	_, err := svc.Register(context.Background(), dto.RegisterInput{
		Email:       "user@example.com",
		Password:    "short",
		DisplayName: "User",
	})
	if !errors.Is(err, service.AuthErrInvalidInput) {
		t.Fatalf("expected AuthErrInvalidInput, got %v", err)
	}
}

func TestRegisterSuccess(t *testing.T) {
	repo := &fakeauth.Repo{}
	svc := service.NewAuthService(repo)
	user, err := svc.Register(context.Background(), dto.RegisterInput{
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
	repo := &fakeauth.Repo{}
	svc := service.NewAuthService(repo)
	_, _ = svc.Register(context.Background(), dto.RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	_, err := svc.Login(context.Background(), dto.LoginInput{
		Email:    "user@example.com",
		Password: "WrongPass!",
	})
	if !errors.Is(err, service.AuthErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	repo := &fakeauth.Repo{}
	svc := service.NewAuthService(repo)
	_, err := svc.Login(context.Background(), dto.LoginInput{
		Email:    "nobody@example.com",
		Password: "Passw0rd!",
	})
	if !errors.Is(err, service.AuthErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginSuccess(t *testing.T) {
	repo := &fakeauth.Repo{}
	svc := service.NewAuthService(repo)
	_, _ = svc.Register(context.Background(), dto.RegisterInput{
		Email:       "user@example.com",
		Password:    "Passw0rd!",
		DisplayName: "User",
	})
	user, err := svc.Login(context.Background(), dto.LoginInput{
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
