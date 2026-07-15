package security

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
	Now      func() time.Time
}

type JWT struct {
	secret   []byte
	issuer   string
	audience string
	now      func() time.Time
}

var _ domain.TokenIssuer = (*JWT)(nil)

type claims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func NewJWT(config JWTConfig) (*JWT, error) {
	if len(config.secretBytes()) < 32 {
		return nil, errors.New("JWT secret must be at least 32 bytes")
	}
	if strings.TrimSpace(config.Issuer) == "" {
		return nil, errors.New("JWT issuer is required")
	}
	if strings.TrimSpace(config.Audience) == "" {
		return nil, errors.New("JWT audience is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &JWT{
		secret:   config.secretBytes(),
		issuer:   config.Issuer,
		audience: config.Audience,
		now:      config.Now,
	}, nil
}

func (j *JWT) Issue(accessClaims domain.AccessTokenClaims) (string, error) {
	if accessClaims.UserID <= 0 || accessClaims.SessionID <= 0 || strings.TrimSpace(accessClaims.TokenID) == "" {
		return "", errors.New("JWT claims require user ID, session ID, and token ID")
	}
	now := j.now().UTC()
	expiresAt := now.Add(domain.AccessTokenLifetime)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		SessionID: strconv.FormatInt(accessClaims.SessionID, 10),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Subject:   strconv.FormatInt(accessClaims.UserID, 10),
			Audience:  jwt.ClaimStrings{j.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        accessClaims.TokenID,
		},
	})
	return token.SignedString(j.secret)
}

func (j *JWT) Parse(raw string) (domain.AccessTokenClaims, error) {
	parsedClaims := new(claims)
	token, err := jwt.ParseWithClaims(
		raw,
		parsedClaims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method %q", token.Method.Alg())
			}
			return j.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(j.issuer),
		jwt.WithAudience(j.audience),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(j.now),
	)
	if err != nil || token == nil || !token.Valid {
		if err == nil {
			err = errors.New("invalid JWT")
		}
		return domain.AccessTokenClaims{}, err
	}
	userID, err := strconv.ParseInt(parsedClaims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return domain.AccessTokenClaims{}, errors.New("invalid JWT subject")
	}
	sessionID, err := strconv.ParseInt(parsedClaims.SessionID, 10, 64)
	if err != nil || sessionID <= 0 {
		return domain.AccessTokenClaims{}, errors.New("invalid JWT session")
	}
	if strings.TrimSpace(parsedClaims.ID) == "" || parsedClaims.IssuedAt == nil || parsedClaims.NotBefore == nil || parsedClaims.ExpiresAt == nil {
		return domain.AccessTokenClaims{}, errors.New("missing JWT claims")
	}
	return domain.AccessTokenClaims{
		UserID:    userID,
		SessionID: sessionID,
		TokenID:   parsedClaims.ID,
		IssuedAt:  parsedClaims.IssuedAt.Time.UTC(),
		NotBefore: parsedClaims.NotBefore.Time.UTC(),
		ExpiresAt: parsedClaims.ExpiresAt.Time.UTC(),
	}, nil
}

func (config JWTConfig) secretBytes() []byte {
	return []byte(strings.TrimSpace(config.Secret))
}
