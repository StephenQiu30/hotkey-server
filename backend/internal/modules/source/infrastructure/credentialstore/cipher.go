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

const (
	currentKeyVersion = 1
	maximumKeyVersion = 32767
)

type Cipher struct {
	currentVersion int
	keys           map[int]cipher.AEAD
}

type SealedCredential struct {
	KeyVersion int
	Nonce      []byte
	Ciphertext []byte
}

func (value *Cipher) supportsKeyVersion(version int) bool {
	return value != nil && value.keys[version] != nil
}

func NewCipher(encodedKey string) (*Cipher, error) {
	return NewCipherKeyring(currentKeyVersion, encodedKey, nil)
}

func NewCipherKeyring(currentVersion int, encodedKey string, previous map[int]string) (*Cipher, error) {
	if currentVersion < 1 || currentVersion > maximumKeyVersion {
		return nil, errors.New("source credential key version is invalid")
	}
	keys := make(map[int]cipher.AEAD, len(previous)+1)
	current, err := newAEAD(encodedKey)
	if err != nil {
		return nil, err
	}
	keys[currentVersion] = current
	for version, material := range previous {
		if version < 1 || version > maximumKeyVersion || version == currentVersion {
			return nil, errors.New("source credential previous key version is invalid")
		}
		value, err := newAEAD(material)
		if err != nil {
			return nil, err
		}
		keys[version] = value
	}
	return &Cipher{currentVersion: currentVersion, keys: keys}, nil
}

func newAEAD(encodedKey string) (cipher.AEAD, error) {
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
	return aead, nil
}

func (value *Cipher) Encrypt(sourceID int64, plaintext []byte) (SealedCredential, error) {
	if value == nil || sourceID <= 0 || len(plaintext) == 0 {
		return SealedCredential{}, errors.New("source credential encryption input is invalid")
	}
	aead := value.keys[value.currentVersion]
	if aead == nil {
		return SealedCredential{}, errors.New("source credential encryption key is unavailable")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SealedCredential{}, fmt.Errorf("generate source credential nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, credentialAAD(sourceID, value.currentVersion))
	return SealedCredential{KeyVersion: value.currentVersion, Nonce: nonce, Ciphertext: sealed}, nil
}

func (value *Cipher) Decrypt(sourceID int64, sealed SealedCredential) ([]byte, error) {
	if value == nil || sourceID <= 0 {
		return nil, errors.New("source credential record is invalid")
	}
	aead := value.keys[sealed.KeyVersion]
	if aead == nil || len(sealed.Nonce) != aead.NonceSize() || len(sealed.Ciphertext) <= aead.Overhead() {
		return nil, errors.New("source credential record is invalid")
	}
	plaintext, err := aead.Open(nil, sealed.Nonce, sealed.Ciphertext, credentialAAD(sourceID, sealed.KeyVersion))
	if err != nil {
		return nil, errors.New("source credential authentication failed")
	}
	return plaintext, nil
}

func credentialAAD(sourceID int64, keyVersion int) []byte {
	return []byte(fmt.Sprintf("hotkey:source-credential:%d:v%d", sourceID, keyVersion))
}
