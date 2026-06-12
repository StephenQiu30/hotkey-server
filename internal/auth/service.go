package auth

import (
	"context"

	"golang.org/x/crypto/bcrypt"
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
func (s *Service) Register(ctx context.Context, input RegisterInput) (User, error) {
	if input.Email == "" || input.Password == "" || input.DisplayName == "" {
		return User{}, ErrInvalidInput
	}
	if len(input.Password) < 8 {
		return User{}, ErrInvalidInput
	}

	if s.repo.ExistsByEmail(ctx, input.Email) {
		return User{}, ErrEmailExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	return s.repo.Create(ctx, input.Email, string(hash), input.DisplayName)
}

// Login authenticates a user by email and password.
func (s *Service) Login(ctx context.Context, input LoginInput) (User, error) {
	user, err := s.repo.GetByEmail(ctx, input.Email)
	if err != nil {
		return User{}, err
	}
	if user == nil {
		return User{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return User{}, ErrInvalidCredentials
	}

	return *user, nil
}
