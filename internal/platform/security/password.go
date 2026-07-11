// Package security provides password hashing, validation, email normalization,
// cryptographic digests, and JWT token operations.
package security

import (
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong  = errors.New("password must be at most 64 characters")
	ErrPasswordBytes    = errors.New("password must be at most 72 UTF-8 bytes")
	ErrPasswordNoLetter = errors.New("password must contain at least one ASCII letter")
	ErrPasswordNoDigit  = errors.New("password must contain at least one digit")
)

// NormalizeEmail parses, lower-cases, and trims an email address.
func NormalizeEmail(email string) (string, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return "", err
	}
	return strings.ToLower(a.Address), nil
}

// ValidatePassword checks password strength requirements:
//   - 8-64 Unicode characters (rune count)
//   - At most 72 UTF-8 bytes (bcrypt limit)
//   - At least one ASCII letter and one digit
//
// Passwords are never normalized or trimmed.
func ValidatePassword(password string) error {
	runes := utf8.RuneCountInString(password)
	if runes < 8 {
		return ErrPasswordTooShort
	}
	if runes > 64 {
		return ErrPasswordTooLong
	}
	if len(password) > 72 {
		return ErrPasswordBytes
	}

	hasLetter := false
	hasDigit := false
	for _, r := range password {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter {
		return ErrPasswordNoLetter
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	return nil
}

// HashPassword returns a bcrypt hash of the password at DefaultCost.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ComparePassword compares a bcrypt hashed password with a plaintext candidate.
// Returns nil on match or an error on mismatch.
func ComparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
