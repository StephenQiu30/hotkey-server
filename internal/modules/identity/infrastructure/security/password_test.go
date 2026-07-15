package security

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHasherHashesAndComparesValidPassword(t *testing.T) {
	t.Parallel()

	hasher := NewPasswordHasher()
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("Hash() returned the cleartext password")
	}
	if err := hasher.Compare(hash, "correct horse battery staple"); err != nil {
		t.Errorf("Compare() error = %v", err)
	}
}

func TestPasswordHasherRejectsNonUTF8AndOver72Bytes(t *testing.T) {
	t.Parallel()

	hasher := NewPasswordHasher()
	for _, password := range []string{string([]byte{0xff}), strings.Repeat("a", 73)} {
		if _, err := hasher.Hash(password); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("Hash(%q) error = %v, want ErrInvalidPassword", password, err)
		}
	}
}
