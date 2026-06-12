package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailExists        = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type RegisterInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginOutput struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type Service struct {
	repo      Repository
	jwtSecret string
}

func NewService(repo Repository, jwtSecret string) *Service {
	return &Service{repo: repo, jwtSecret: jwtSecret}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (User, error) {
	if s.repo.ExistsByEmail(ctx, input.Email) {
		return User{}, ErrEmailExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	return s.repo.Create(ctx, input.Email, string(hash), input.DisplayName)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginOutput, error) {
	user, hash, err := s.repo.FindByEmail(ctx, input.Email)
	if err != nil {
		return LoginOutput{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)); err != nil {
		return LoginOutput{}, ErrInvalidCredentials
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return LoginOutput{}, err
	}

	return LoginOutput{Token: tokenStr, User: user}, nil
}
