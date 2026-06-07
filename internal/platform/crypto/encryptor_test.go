package crypto_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
)

func TestAESGCMEncryptor_RoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}

	plaintext := "ghp_test_token_12345"
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext == plaintext {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestAESGCMEncryptor_InvalidKey(t *testing.T) {
	shortKey := []byte("short")
	_, err := crypto.NewAESGCMEncryptor(shortKey)
	if err != crypto.ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestAESGCMEncryptor_DecryptInvalidData(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}

	_, err = enc.Decrypt("not-valid-base64!!!")
	if err != crypto.ErrDecryptionFailed {
		t.Fatalf("expected ErrDecryptionFailed, got %v", err)
	}
}
