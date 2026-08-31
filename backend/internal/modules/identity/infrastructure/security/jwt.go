package security

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	Secret         string
	KeyID          string
	PreviousSecret string
	PreviousKeyID  string
	Issuer         string
	Audience       string
	Now            func() time.Time
}

type JWT struct {
	secret         []byte
	keyID          string
	previousSecret []byte
	previousKeyID  string
	issuer         string
	audience       string
	now            func() time.Time
}

var _ domain.TokenIssuer = (*JWT)(nil)

type claims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

var jwtKeyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

func NewJWT(config JWTConfig) (*JWT, error) {
	if len(config.secretBytes()) < 32 {
		return nil, errors.New("JWT secret must be at least 32 bytes")
	}
	config.KeyID = strings.TrimSpace(config.KeyID)
	config.PreviousKeyID = strings.TrimSpace(config.PreviousKeyID)
	previousSecret := []byte(strings.TrimSpace(config.PreviousSecret))
	if config.KeyID != "" && !jwtKeyIDPattern.MatchString(config.KeyID) {
		return nil, errors.New("JWT key ID is invalid")
	}
	if len(previousSecret) > 0 {
		if len(previousSecret) < 32 {
			return nil, errors.New("previous JWT secret must be at least 32 bytes")
		}
		if config.KeyID == "" || !jwtKeyIDPattern.MatchString(config.PreviousKeyID) || config.PreviousKeyID == config.KeyID {
			return nil, errors.New("JWT rotation key IDs are invalid")
		}
		if subtle.ConstantTimeCompare(config.secretBytes(), previousSecret) == 1 {
			return nil, errors.New("JWT rotation keys must be distinct")
		}
	} else if config.PreviousKeyID != "" {
		return nil, errors.New("previous JWT key ID requires a previous secret")
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
		secret:         config.secretBytes(),
		keyID:          config.KeyID,
		previousSecret: previousSecret,
		previousKeyID:  config.PreviousKeyID,
		issuer:         config.Issuer,
		audience:       config.Audience,
		now:            config.Now,
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
	if j.keyID != "" {
		token.Header["kid"] = j.keyID
	}
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
			keyID, _ := token.Header["kid"].(string)
			switch {
			case keyID == "" && len(j.previousSecret) > 0:
				// Tokens issued before Key IDs were introduced are accepted only
				// through the explicitly configured previous-key window.
				return j.previousSecret, nil
			case keyID == "":
				return j.secret, nil
			case keyID == j.keyID:
				return j.secret, nil
			case len(j.previousSecret) > 0 && keyID == j.previousKeyID:
				return j.previousSecret, nil
			default:
				return nil, errors.New("JWT key ID is not accepted")
			}
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
		IssuedAt:  parsedClaims.IssuedAt.UTC(),
		NotBefore: parsedClaims.NotBefore.UTC(),
		ExpiresAt: parsedClaims.ExpiresAt.UTC(),
	}, nil
}

func (config JWTConfig) secretBytes() []byte {
	return []byte(strings.TrimSpace(config.Secret))
}
