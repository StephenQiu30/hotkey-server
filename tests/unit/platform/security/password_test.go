package security_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
)

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		// valid
		{name: "valid ascii", value: "example123", valid: true},
		{name: "valid with mixed case", value: "MyP@ss123", valid: true},
		{name: "valid 8 chars exactly", value: "abc12345", valid: true},
		{name: "valid 64 chars exactly", value: strings.Repeat("a", 62) + "b1", valid: true},  // 64 chars, 64 bytes
		{name: "valid near byte limit", value: strings.Repeat("你", 22) + "a1", valid: true},   // 22*3+2=68 bytes, 24 runes
		// invalid
		{name: "no digit", value: "abcdefgh", valid: false},
		{name: "no letter", value: "12345678", valid: false},
		{name: "too short", value: "abc123", valid: false},
		{name: "too many runes (65)", value: strings.Repeat("你", 64) + "a1", valid: false}, // 65 runes > 64, also exceeds 72 bytes
		{name: "bcrypt bytes", value: strings.Repeat("你", 25) + "a1", valid: false},         // 25*3+2=77 bytes > 72
		{name: "empty password", value: "", valid: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := security.ValidatePassword(tc.value)
			got := err == nil
			if got != tc.valid {
				t.Fatalf("ValidatePassword(%q) err=%v, expected valid=%v", tc.value, err, tc.valid)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "simple", input: "user@example.com", want: "user@example.com"},
		{name: "uppercase", input: "User@Example.COM", want: "user@example.com"},
		{name: "with spaces", input: "  User@Example.COM  ", want: "user@example.com"},
		{name: "with plus", input: "user+tag@example.com", want: "user+tag@example.com"},
		{name: "invalid email", input: "not-an-email", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := security.NormalizeEmail(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeEmail(%q) expected error, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeEmail(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeEmail(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestHashAndComparePassword(t *testing.T) {
	password := "MySecureP@ss123"

	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword() returned empty hash")
	}
	if hash == password {
		t.Fatal("HashPassword() returned plaintext password")
	}

	// Compare correct password
	err = security.ComparePassword(hash, password)
	if err != nil {
		t.Fatalf("ComparePassword() with correct password: %v", err)
	}

	// Compare incorrect password
	err = security.ComparePassword(hash, "wrongpassword123")
	if err == nil {
		t.Fatal("ComparePassword() expected error for wrong password")
	}

	// Hashes should be different each time
	hash2, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() second call unexpected error: %v", err)
	}
	if hash == hash2 {
		t.Fatal("HashPassword() should produce different salt each time")
	}
}
