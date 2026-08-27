package domain

import (
	"errors"
	"testing"
)

func TestNormalizeEmailTrimsAndLowercases(t *testing.T) {
	t.Parallel()

	email, err := NormalizeEmail("  Admin@Example.COM \t")
	if err != nil {
		t.Fatalf("NormalizeEmail() error = %v", err)
	}
	if email != "admin@example.com" {
		t.Errorf("NormalizeEmail() = %q, want admin@example.com", email)
	}
}

func TestNormalizeEmailRejectsMalformedAddresses(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"",
		"invalid",
		"missing-domain@",
		"@missing-local.test",
		"Display Name <reader@example.test>",
		"reader @example.test",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeEmail(raw); !errors.Is(err, ErrInvalidEmail) {
				t.Fatalf("NormalizeEmail(%q) error = %v, want ErrInvalidEmail", raw, err)
			}
		})
	}
}

func TestRoleIsLimitedToSupportedValues(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleAdmin, RoleAnalyst, RoleEditor, RoleViewer} {
		if !role.Valid() {
			t.Errorf("role %q is not valid", role)
		}
	}
	if Role("owner").Valid() {
		t.Fatal("unsupported role is valid")
	}
}
