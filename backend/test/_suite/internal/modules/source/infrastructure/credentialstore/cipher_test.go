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
