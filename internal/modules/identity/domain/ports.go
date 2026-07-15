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
	LockByID(context.Context, int64) (*User, error)
	LockActiveAdmins(context.Context) ([]User, error)
	Create(context.Context, *User) error
	UpdatePassword(context.Context, int64, string, time.Time) error
	TouchLogin(context.Context, int64, time.Time) error
	ChangeRole(context.Context, int64, Role, time.Time) (*User, error)
	ChangeStatus(context.Context, int64, UserStatus, time.Time) (*User, error)
	SoftDelete(context.Context, int64, time.Time) (*User, error)
	RestoreDisabled(context.Context, int64, time.Time) (*User, error)
}

type SessionRepository interface {
	Create(context.Context, *Session, *RefreshToken) error
	FindByRefreshTokenHash(context.Context, string) (*Session, *RefreshToken, error)
	Rotate(context.Context, string, *RefreshToken, time.Time) (*Session, *RefreshToken, error)
	RevokeSession(context.Context, int64, string, time.Time) error
	RevokeAllForUser(context.Context, int64, string, time.Time) error
}

// AuditEntry contains only safe actor/resource facts and a caller-supplied
// state delta. Infrastructure applies a final allowlist before persistence.
type AuditEntry struct {
	ActorType    string
	ActorID      int64
	Action       string
	ResourceType string
	ResourceID   int64
	RequestID    string
	TraceID      string
	BeforeData   map[string]any
	AfterData    map[string]any
	Result       string
	IPHash       string
}

type AuditRepository interface {
	Create(context.Context, AuditEntry) error
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
