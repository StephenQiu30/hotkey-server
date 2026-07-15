package domain

import "testing"

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

func TestRoleIsLimitedToSupportedValues(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleAdmin, RoleEditor, RoleViewer} {
		if !role.Valid() {
			t.Errorf("role %q is not valid", role)
		}
	}
	if Role("owner").Valid() {
		t.Fatal("unsupported role is valid")
	}
}
