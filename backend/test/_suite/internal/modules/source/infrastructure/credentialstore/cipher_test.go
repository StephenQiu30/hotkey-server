package credentialstore

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestCipherEncryptsAuthenticatesAndBindsSourceIdentity(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	plaintext := []byte("provider-token-value")
	sealed, err := cipher.Encrypt(42, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(sealed.Nonce) != 12 || bytes.Contains(sealed.Ciphertext, plaintext) {
		t.Fatalf("sealed credential is not an opaque AES-GCM record: %#v", sealed)
	}
	opened, err := cipher.Decrypt(42, sealed)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("Decrypt() = %q, %v", opened, err)
	}
	if _, err := cipher.Decrypt(43, sealed); err == nil {
		t.Fatal("Decrypt() accepted ciphertext for another source")
	}
	sealed.Ciphertext[0] ^= 0xff
	if _, err := cipher.Decrypt(42, sealed); err == nil {
		t.Fatal("Decrypt() accepted tampered ciphertext")
	}
}

func TestCipherRejectsMissingMalformedAndWrongLengthKeys(t *testing.T) {
	for _, key := range []string{"", "not-base64", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16))} {
		if _, err := NewCipher(key); err == nil {
			t.Fatalf("NewCipher(%q) accepted invalid key", key)
		}
	}
}

func TestCipherKeyringWritesCurrentVersionReadsPreviousAndRejectsItAfterRevocation(t *testing.T) {
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	legacy, err := NewCipherKeyring(1, oldKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyRecord, err := legacy.Encrypt(42, []byte("old-provider-token"))
	if err != nil {
		t.Fatal(err)
	}

	rolling, err := NewCipherKeyring(2, newKey, map[int]string{1: oldKey})
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := rolling.Decrypt(42, legacyRecord); err != nil || string(opened) != "old-provider-token" {
		t.Fatalf("rolling decrypt legacy = %q, %v", opened, err)
	}
	currentRecord, err := rolling.Encrypt(42, []byte("new-provider-token"))
	if err != nil {
		t.Fatal(err)
	}
	if currentRecord.KeyVersion != 2 {
		t.Fatalf("current record key version = %d, want 2", currentRecord.KeyVersion)
	}
	if _, err := legacy.Decrypt(42, currentRecord); err == nil {
		t.Fatal("legacy-only keyring decrypted current ciphertext")
	}

	revoked, err := NewCipherKeyring(2, newKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revoked.Decrypt(42, legacyRecord); err == nil {
		t.Fatal("revoked keyring decrypted legacy ciphertext")
	}
	if opened, err := revoked.Decrypt(42, currentRecord); err != nil || string(opened) != "new-provider-token" {
		t.Fatalf("revoked keyring decrypt current = %q, %v", opened, err)
	}
}

func TestCipherKeyringRejectsDuplicateOrOutOfRangeVersionsWithoutLeakingKeys(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))
	for _, test := range []struct {
		version  int
		previous map[int]string
	}{
		{version: 0},
		{version: 32768},
		{version: 2, previous: map[int]string{2: key}},
	} {
		_, err := NewCipherKeyring(test.version, key, test.previous)
		if err == nil {
			t.Fatal("NewCipherKeyring() accepted invalid version configuration")
		}
		if bytes.Contains([]byte(err.Error()), []byte(key)) {
			t.Fatalf("NewCipherKeyring() leaked key material: %v", err)
		}
	}
}
