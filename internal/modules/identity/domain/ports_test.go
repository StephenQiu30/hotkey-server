package domain

import "testing"

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

var (
	_ UserRepository
	_ SessionRepository
	_ PasswordHasher
	_ TokenIssuer
	_ VerificationStore
	_ Mailer
	_ Clock
)
