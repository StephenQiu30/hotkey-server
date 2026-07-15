package security

import (
	"errors"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidPassword = errors.New("password must be valid UTF-8 and at most 72 bytes")

type PasswordHasher struct{}

var _ domain.PasswordHasher = PasswordHasher{}

func NewPasswordHasher() PasswordHasher {
	return PasswordHasher{}
}

func (PasswordHasher) Hash(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (PasswordHasher) Compare(hash, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func validatePassword(password string) error {
	if !utf8.ValidString(password) || len(password) > 72 {
		return ErrInvalidPassword
	}
	return nil
}
