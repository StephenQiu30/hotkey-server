package security

import (
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWTRotationSignsOnlyWithCurrentKeyAndAcceptsLegacyUntilRevoked(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 28, 18, 0, 0, 0, time.UTC)
	oldSecret := strings.Repeat("o", 32)
	newSecret := strings.Repeat("n", 32)
	legacy, err := NewJWT(JWTConfig{
		Secret: oldSecret, Issuer: "hotkey", Audience: "hotkey-web", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyToken, err := legacy.Issue(domain.AccessTokenClaims{UserID: 42, SessionID: 9, TokenID: "legacy-token"})
	if err != nil {
		t.Fatal(err)
	}

	rolling, err := NewJWT(JWTConfig{
		Secret: newSecret, KeyID: "jwt-v2", PreviousKeyID: "jwt-v1", PreviousSecret: oldSecret,
		Issuer: "hotkey", Audience: "hotkey-web", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	currentToken, err := rolling.Issue(domain.AccessTokenClaims{UserID: 42, SessionID: 9, TokenID: "current-token"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, err := new(jwt.Parser).ParseUnverified(currentToken, jwt.MapClaims{})
	if err != nil || parsed.Header["kid"] != "jwt-v2" {
		t.Fatalf("current token key ID = %#v, error = %v", parsed.Header["kid"], err)
	}
	if _, err := rolling.Parse(legacyToken); err != nil {
		t.Fatalf("rolling verifier rejected legacy token: %v", err)
	}
	if _, err := rolling.Parse(currentToken); err != nil {
		t.Fatalf("rolling verifier rejected current token: %v", err)
	}

	revoked, err := NewJWT(JWTConfig{
		Secret: newSecret, KeyID: "jwt-v2", Issuer: "hotkey", Audience: "hotkey-web", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revoked.Parse(legacyToken); err == nil {
		t.Fatal("revoked verifier accepted legacy token")
	}
	if _, err := revoked.Parse(currentToken); err != nil {
		t.Fatalf("revoked verifier rejected current token: %v", err)
	}
}

func TestJWTRotationRejectsAmbiguousOrUnsafeKeyConfigurationWithoutLeakingSecrets(t *testing.T) {
	t.Parallel()

	oldSecret := strings.Repeat("old-sensitive-", 3)
	newSecret := strings.Repeat("new-sensitive-", 3)
	for _, config := range []JWTConfig{
		{Secret: newSecret, KeyID: "same", PreviousKeyID: "same", PreviousSecret: oldSecret, Issuer: "hotkey", Audience: "hotkey-web"},
		{Secret: newSecret, KeyID: "jwt-v2", PreviousKeyID: "jwt-v1", PreviousSecret: newSecret, Issuer: "hotkey", Audience: "hotkey-web"},
		{Secret: newSecret, KeyID: "invalid key id", Issuer: "hotkey", Audience: "hotkey-web"},
	} {
		_, err := NewJWT(config)
		if err == nil {
			t.Fatal("NewJWT() accepted ambiguous rotation configuration")
		}
		if strings.Contains(err.Error(), oldSecret) || strings.Contains(err.Error(), newSecret) {
			t.Fatalf("NewJWT() leaked key material: %v", err)
		}
	}
}

func TestJWTIssuesAndParsesHS256Token(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	issuer, err := NewJWT(JWTConfig{
		Secret:   "0123456789abcdef0123456789abcdef",
		Issuer:   "hotkey",
		Audience: "hotkey-web",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewJWT() error = %v", err)
	}

	raw, err := issuer.Issue(domain.AccessTokenClaims{UserID: 42, SessionID: 9, TokenID: "token-1"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := issuer.Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.UserID != 42 || claims.SessionID != 9 || claims.TokenID != "token-1" {
		t.Errorf("Parse() = %#v, want issued identity", claims)
	}
	if got, want := claims.ExpiresAt, now.Add(15*time.Minute); !got.Equal(want) {
		t.Errorf("ExpiresAt = %s, want %s", got, want)
	}
}

func TestJWTRejectsNonHS256WrongIssuerAudienceAndExpiredTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	config := JWTConfig{
		Secret:   "0123456789abcdef0123456789abcdef",
		Issuer:   "hotkey",
		Audience: "hotkey-web",
		Now:      func() time.Time { return now },
	}
	issuer, err := NewJWT(config)
	if err != nil {
		t.Fatalf("NewJWT() error = %v", err)
	}

	for _, tt := range []struct {
		name   string
		method jwt.SigningMethod
		claims jwt.RegisteredClaims
	}{
		{
			name:   "non HS256",
			method: jwt.SigningMethodHS384,
			claims: registeredClaims(now, "hotkey", "hotkey-web"),
		},
		{
			name:   "wrong issuer",
			method: jwt.SigningMethodHS256,
			claims: registeredClaims(now, "other", "hotkey-web"),
		},
		{
			name:   "wrong audience",
			method: jwt.SigningMethodHS256,
			claims: registeredClaims(now, "hotkey", "other"),
		},
		{
			name:   "expired",
			method: jwt.SigningMethodHS256,
			claims: func() jwt.RegisteredClaims {
				claims := registeredClaims(now, "hotkey", "hotkey-web")
				claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second))
				return claims
			}(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := jwt.NewWithClaims(tt.method, jwt.MapClaims{
				"sub": tt.claims.Subject,
				"sid": "9",
				"jti": tt.claims.ID,
				"iss": tt.claims.Issuer,
				"aud": []string(tt.claims.Audience),
				"iat": tt.claims.IssuedAt.Unix(),
				"nbf": tt.claims.NotBefore.Unix(),
				"exp": tt.claims.ExpiresAt.Unix(),
			}).SignedString(config.secretBytes())
			if err != nil {
				t.Fatalf("SignedString() error = %v", err)
			}
			if _, err := issuer.Parse(raw); err == nil {
				t.Fatal("Parse() error = nil, want rejected token")
			}
		})
	}
}

func registeredClaims(now time.Time, issuer, audience string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   "42",
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ID:        "token-1",
	}
}
