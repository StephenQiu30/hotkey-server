package security_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
)

func TestNewRefreshToken(t *testing.T) {
	token, hash := security.NewRefreshToken()

	if len(token) == 0 {
		t.Fatal("NewRefreshToken() returned empty token")
	}
	if len(hash) == 0 {
		t.Fatal("NewRefreshToken() returned empty hash")
	}

	// Token should be hex of 32 bytes = 64 hex chars
	if len(token) != 64 {
		t.Fatalf("NewRefreshToken() token length = %d, want 64 (hex of 32 bytes)", len(token))
	}

	// SHA256 hash should be 64 hex chars
	if len(hash) != 64 {
		t.Fatalf("NewRefreshToken() hash length = %d, want 64 (SHA256 hex)", len(hash))
	}

	// Subsequent calls should produce different tokens
	token2, hash2 := security.NewRefreshToken()
	if token == token2 {
		t.Fatal("NewRefreshToken() should produce unique tokens")
	}
	if hash == hash2 {
		t.Fatal("NewRefreshToken() should produce unique hashes")
	}
}

func TestHMACDigest(t *testing.T) {
	data := "hello"
	secret := "mysecret"

	digest1 := security.HMACDigest(data, secret)
	digest2 := security.HMACDigest(data, secret)
	digest3 := security.HMACDigest(data, "different-secret")
	digest4 := security.HMACDigest("different-data", secret)

	if digest1 == "" {
		t.Fatal("HMACDigest() returned empty")
	}

	// Deterministic with same inputs
	if digest1 != digest2 {
		t.Fatal("HMACDigest() should be deterministic with same inputs")
	}

	// Different secret, different output
	if digest1 == digest3 {
		t.Fatal("HMACDigest() should differ with different secret")
	}

	// Different data, different output
	if digest1 == digest4 {
		t.Fatal("HMACDigest() should differ with different data")
	}
}

func TestSHA256Digest(t *testing.T) {
	data := "hello"
	digest1 := security.SHA256Digest(data)
	digest2 := security.SHA256Digest(data)
	digest3 := security.SHA256Digest("world")

	if digest1 == "" {
		t.Fatal("SHA256Digest() returned empty")
	}

	// Deterministic with same input
	if digest1 != digest2 {
		t.Fatal("SHA256Digest() should be deterministic with same input")
	}

	// Different for different input
	if digest1 == digest3 {
		t.Fatal("SHA256Digest() should differ for different input")
	}
}

func TestSignAndParseAccessToken(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	sessionID := int64(42)

	tokenStr, err := security.SignAccessToken(security.AccessClaims{
		SessionID: sessionID,
	}, secret, "hotkey-server", "hotkey-web")
	if err != nil {
		t.Fatalf("SignAccessToken() unexpected error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("SignAccessToken() returned empty token")
	}

	// Parse valid token
	claims, err := security.ParseAccessToken(tokenStr, secret, "hotkey-server", "hotkey-web")
	if err != nil {
		t.Fatalf("ParseAccessToken() unexpected error: %v", err)
	}
	if claims == nil {
		t.Fatal("ParseAccessToken() returned nil claims")
	}
	if claims.SessionID != sessionID {
		t.Fatalf("ParseAccessToken() SessionID = %d, want %d", claims.SessionID, sessionID)
	}

	// Verify registered claims
	if claims.Issuer != "hotkey-server" {
		t.Fatalf("ParseAccessToken() claims.Issuer = %q, want %q", claims.Issuer, "hotkey-server")
	}

	// Check audience contains "hotkey-web"
	if len(claims.Audience) != 1 || claims.Audience[0] != "hotkey-web" {
		t.Fatalf("ParseAccessToken() claims.Audience = %v, want [%q]", claims.Audience, "hotkey-web")
	}

	// Check expiry is ~15 minutes
	expectedExp := claims.IssuedAt.Time.Add(15 * time.Minute)
	expDiff := claims.ExpiresAt.Time.Sub(expectedExp)
	if expDiff.Abs() > time.Second {
		t.Fatalf("ParseAccessToken() expiry diff = %v, want ~0", expDiff)
	}
}

func TestParseAccessTokenWrongSecret(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	wrongSecret := "wrong-secret-key-for-jwt"

	tokenStr, err := security.SignAccessToken(security.AccessClaims{
		SessionID: 1,
	}, secret, "hotkey-server", "hotkey-web")
	if err != nil {
		t.Fatalf("SignAccessToken() unexpected error: %v", err)
	}

	_, err = security.ParseAccessToken(tokenStr, wrongSecret, "hotkey-server", "hotkey-web")
	if err == nil {
		t.Fatal("ParseAccessToken() expected error for wrong secret")
	}
}

func TestParseAccessTokenWrongIssuer(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	sessionID := int64(1)

	// Create token with wrong issuer directly
	tokenStr := signRawToken(t, secret, sessionID, "wrong-issuer", "hotkey-web", time.Now().Add(15*time.Minute))
	_, err := security.ParseAccessToken(tokenStr, secret, "hotkey-server", "hotkey-web")
	if err == nil {
		t.Fatal("ParseAccessToken() expected error for wrong issuer")
	}
}

func TestParseAccessTokenWrongAudience(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	sessionID := int64(1)

	tokenStr := signRawToken(t, secret, sessionID, "hotkey-server", "wrong-audience", time.Now().Add(15*time.Minute))
	_, err := security.ParseAccessToken(tokenStr, secret, "hotkey-server", "hotkey-web")
	if err == nil {
		t.Fatal("ParseAccessToken() expected error for wrong audience")
	}
}

func TestParseAccessTokenExpired(t *testing.T) {
	secret := "test-secret-key-for-jwt"

	tokenStr := signRawToken(t, secret, 1, "hotkey-server", "hotkey-web", time.Now().Add(-1*time.Hour))
	_, err := security.ParseAccessToken(tokenStr, secret, "hotkey-server", "hotkey-web")
	if err == nil {
		t.Fatal("ParseAccessToken() expected error for expired token")
	}
}

func TestParseAccessTokenWrongAlgorithm(t *testing.T) {
	secret := "test-secret-key-for-jwt"

	// Use a different signing algorithm
	claims := security.AccessClaims{
		SessionID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "hotkey-server",
			Audience:  jwt.ClaimStrings{"hotkey-web"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString() unexpected error: %v", err)
	}

	_, err = security.ParseAccessToken(tokenStr, secret, "hotkey-server", "hotkey-web")
	if err == nil {
		t.Fatal("ParseAccessToken() expected error for wrong algorithm")
	}
}

// signRawToken creates a raw JWT with the given parameters, bypassing SignAccessToken.
func signRawToken(t *testing.T, secret string, sessionID int64, issuer, audience string, expiresAt time.Time) string {
	t.Helper()
	claims := security.AccessClaims{
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString() unexpected error: %v", err)
	}
	return tokenStr
}
