package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// HMACDigest computes HMAC-SHA256 of data using the given secret and returns
// the hex-encoded digest.
func HMACDigest(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

// SHA256Digest returns the hex-encoded SHA-256 hash of data.
func SHA256Digest(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// NewRefreshToken generates a cryptographically random 32-byte token.
// It returns the hex-encoded token and its SHA-256 hash.
func NewRefreshToken() (token string, hash string) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	token = hex.EncodeToString(b)
	hash = SHA256Digest(token)
	return
}
