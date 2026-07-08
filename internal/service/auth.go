package service

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
	AuthErrInvalidInput   = errors.New("invalid input")
)

// AuthRepository defines the persistence interface for auth operations.
type AuthRepository interface {
	ExistsByEmail(ctx context.Context, email string) bool
	Create(ctx context.Context, email, passwordHash, displayName string) (dto.User, error)
	GetByEmail(ctx context.Context, email string) (*dto.User, error)
	GetByID(ctx context.Context, id int64) (*dto.User, error)
}

// AuthService provides authentication operations.
type AuthService struct {
	repo AuthRepository
}

// NewAuthService creates a new auth Service.
func NewAuthService(repo AuthRepository) *AuthService {
	return &AuthService{repo: repo}
}

// Register creates a new user account.
func (s *AuthService) Register(ctx context.Context, input dto.RegisterInput) (dto.User, error) {
	if input.Email == "" || input.Password == "" || input.DisplayName == "" {
		return dto.User{}, AuthErrInvalidInput
	}
	if len(input.Password) < 8 {
		return dto.User{}, AuthErrInvalidInput
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
func (s *AuthService) Login(ctx context.Context, input dto.LoginInput) (dto.User, error) {
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
