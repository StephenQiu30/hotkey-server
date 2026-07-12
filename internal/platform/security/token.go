package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenIssuer   = "hotkey-server"
	accessTokenAudience = "hotkey-web"
	accessTokenTTL      = 15 * time.Minute
)

// AccessClaims represents the JWT claims for an access token.
type AccessClaims struct {
	SessionID int64 `json:"sid"`
	jwt.RegisteredClaims
}

// SignAccessToken creates a signed JWT access token for the given claims.
// The token is signed with HMAC-SHA256 and includes a 15-minute expiry.
func SignAccessToken(claims AccessClaims, secret, issuer, audience string) (string, error) {
	now := time.Now()
	claims.Issuer = issuer
	claims.Audience = jwt.ClaimStrings{audience}
	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(accessTokenTTL))
	claims.NotBefore = jwt.NewNumericDate(now)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseAccessToken parses and validates a JWT access token string.
// It verifies the issuer, audience, signing method, expiry, and not-before time.
func ParseAccessToken(tokenStr, secret, issuer, audience string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	},
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithLeeway(0),
	)
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
