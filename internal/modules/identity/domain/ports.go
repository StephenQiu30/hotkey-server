package domain

import (
	"context"
	"time"
)

type VerificationPurpose string

const (
	VerificationPurposeRegistration  VerificationPurpose = "registration"
	VerificationPurposePasswordReset VerificationPurpose = "password_reset"
)

func (p VerificationPurpose) Valid() bool {
	return p == VerificationPurposeRegistration || p == VerificationPurposePasswordReset
}

type VerificationTicket struct {
	Token     string
	Email     string
	Purpose   VerificationPurpose
	ExpiresAt time.Time
}

type UserRepository interface {
	FindByEmail(context.Context, string) (*User, error)
	FindByID(context.Context, int64) (*User, error)
	Create(context.Context, *User) error
}

type SessionRepository interface {
	Create(context.Context, *Session, *RefreshToken) error
	FindByRefreshTokenHash(context.Context, string) (*Session, *RefreshToken, error)
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Compare(string, string) error
}

type TokenIssuer interface {
	Issue(AccessTokenClaims) (string, error)
	Parse(string) (AccessTokenClaims, error)
}

type VerificationStore interface {
	CreateCode(context.Context, VerificationPurpose, string, string, time.Time) error
	ConsumeCode(context.Context, VerificationPurpose, string, string) (VerificationTicket, error)
	ConsumeTicket(context.Context, VerificationPurpose, string) (VerificationTicket, error)
}

type Mailer interface {
	SendVerificationCode(context.Context, VerificationPurpose, string, string) error
}

type Clock interface {
	Now() time.Time
}
