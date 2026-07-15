package domain

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVerificationPurposeIsLimitedToRegistrationAndPasswordReset(t *testing.T) {
	t.Parallel()

	for _, purpose := range []VerificationPurpose{VerificationPurposeRegistration, VerificationPurposePasswordReset} {
		if !purpose.Valid() {
			t.Errorf("purpose %q is not valid", purpose)
		}
	}
	if VerificationPurpose("email_change").Valid() {
		t.Fatal("unsupported verification purpose is valid")
	}
}

func TestPersistenceContractsExposeIdentityWorkflowOperations(t *testing.T) {
	t.Parallel()

	var _ interface {
		Rotate(context.Context, string, *RefreshToken, time.Time) (*Session, *RefreshToken, error)
		RevokeSession(context.Context, int64, string, time.Time) error
		RevokeAllForUser(context.Context, int64, string, time.Time) error
	} = (SessionRepository)(nil)
	var _ interface {
		LockByID(context.Context, int64) (*User, error)
		LockActiveAdmins(context.Context) ([]User, error)
	} = (UserRepository)(nil)
	var _ interface {
		Create(context.Context, AuditEntry) error
	} = (AuditRepository)(nil)

	if errors.Is(ErrRefreshReplay, ErrRefreshInvalid) {
		t.Fatal("refresh replay and invalid sentinels must remain distinct")
	}
}

var (
	_ UserRepository
	_ SessionRepository
	_ PasswordHasher
	_ TokenIssuer
	_ VerificationStore
	_ Mailer
	_ AuditRepository
	_ Clock
)
