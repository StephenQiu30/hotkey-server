package credentialstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const currentKeyVersion = 1

type Cipher struct{ aead cipher.AEAD }

type SealedCredential struct {
	KeyVersion int
	Nonce      []byte
	Ciphertext []byte
}

func NewCipher(encodedKey string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, errors.New("source credential master key must be Base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create source credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create source credential AEAD: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func (value *Cipher) Encrypt(sourceID int64, plaintext []byte) (SealedCredential, error) {
	if value == nil || value.aead == nil || sourceID <= 0 || len(plaintext) == 0 {
		return SealedCredential{}, errors.New("source credential encryption input is invalid")
	}
	nonce := make([]byte, value.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SealedCredential{}, fmt.Errorf("generate source credential nonce: %w", err)
	}
	sealed := value.aead.Seal(nil, nonce, plaintext, credentialAAD(sourceID, currentKeyVersion))
	return SealedCredential{KeyVersion: currentKeyVersion, Nonce: nonce, Ciphertext: sealed}, nil
}

func (value *Cipher) Decrypt(sourceID int64, sealed SealedCredential) ([]byte, error) {
	if value == nil || value.aead == nil || sourceID <= 0 || sealed.KeyVersion != currentKeyVersion || len(sealed.Nonce) != value.aead.NonceSize() || len(sealed.Ciphertext) <= value.aead.Overhead() {
		return nil, errors.New("source credential record is invalid")
	}
	plaintext, err := value.aead.Open(nil, sealed.Nonce, sealed.Ciphertext, credentialAAD(sourceID, sealed.KeyVersion))
	if err != nil {
		return nil, errors.New("source credential authentication failed")
	}
	return plaintext, nil
}

func credentialAAD(sourceID int64, keyVersion int) []byte {
	return []byte(fmt.Sprintf("hotkey:source-credential:%d:v%d", sourceID, keyVersion))
}
