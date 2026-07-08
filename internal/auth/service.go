package auth

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors for auth operations.
var (
	ErrEmailExists        = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidInput       = errors.New("invalid input")
)

// Service provides authentication operations.
type Service struct {
	repo Repository
}

// NewService creates a new auth Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Register creates a new user account.
func (s *Service) Register(ctx context.Context, input dto.RegisterInput) (dto.User, error) {
	if input.Email == "" || input.Password == "" || input.DisplayName == "" {
		return dto.User{}, ErrInvalidInput
	}
	if len(input.Password) < 8 {
		return dto.User{}, ErrInvalidInput
	}

	if s.repo.ExistsByEmail(ctx, input.Email) {
		return dto.User{}, ErrEmailExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return dto.User{}, err
	}

	return s.repo.Create(ctx, input.Email, string(hash), input.DisplayName)
}

// Login authenticates a user by email and password.
func (s *Service) Login(ctx context.Context, input dto.LoginInput) (dto.User, error) {
	user, err := s.repo.GetByEmail(ctx, input.Email)
	if err != nil {
		return dto.User{}, err
	}
	if user == nil {
		return dto.User{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return dto.User{}, ErrInvalidCredentials
	}

	return *user, nil
}
