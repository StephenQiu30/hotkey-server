package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrInvalidAccessToken  = errors.New("invalid access token")
	ErrForbidden           = errors.New("forbidden")
)

type Config struct {
	AccessTokenSecret string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
}

type Repository interface {
	CreateUser(ctx context.Context, user user.User) (user.User, error)
	UserByEmail(ctx context.Context, email string) (user.User, error)
	UserByID(ctx context.Context, id string) (user.User, error)
	CreateRefreshToken(ctx context.Context, token RefreshToken) error
	RefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string, revokedAt time.Time) error
}

type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

type RegisterInput struct {
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

type Session struct {
	User         user.User
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type Service struct {
	repo Repository
	cfg  Config
	now  func() time.Time
}

func NewService(repo Repository, cfg Config) (*Service, error) {
	if strings.TrimSpace(cfg.AccessTokenSecret) == "" {
		return nil, errors.New("access token secret is required")
	}
	if cfg.AccessTokenTTL == 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	return &Service{repo: repo, cfg: cfg, now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (user.User, error) {
	email, err := user.NormalizeEmail(input.Email)
	if err != nil {
		return user.User{}, err
	}
	if len(input.Password) < 12 {
		return user.User{}, ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return user.User{}, fmt.Errorf("hash password: %w", err)
	}
	now := s.now().UTC()
	created, err := user.NewEmailUser(newID("usr"), email, string(hash), now)
	if err != nil {
		return user.User{}, err
	}
	created, err = s.repo.CreateUser(ctx, created)
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyExists) {
			return user.User{}, ErrEmailAlreadyExists
		}
		return user.User{}, err
	}
	return created, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (Session, error) {
	email, err := user.NormalizeEmail(input.Email)
	if err != nil {
		return Session{}, ErrInvalidCredentials
	}
	found, err := s.repo.UserByEmail(ctx, email)
	if err != nil {
		return Session{}, ErrInvalidCredentials
	}
	if found.Status != user.StatusActive {
		return Session{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(input.Password)) != nil {
		return Session{}, ErrInvalidCredentials
	}
	return s.issueSession(ctx, found)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (Session, error) {
	tokenHash := hashToken(refreshToken)
	stored, err := s.repo.RefreshTokenByHash(ctx, tokenHash)
	if err != nil || stored.RevokedAt != nil || !stored.ExpiresAt.After(s.now().UTC()) {
		return Session{}, ErrInvalidRefreshToken
	}
	found, err := s.repo.UserByID(ctx, stored.UserID)
	if err != nil || found.Status != user.StatusActive {
		return Session{}, ErrInvalidRefreshToken
	}
	if err := s.repo.RevokeRefreshToken(ctx, stored.TokenHash, s.now().UTC()); err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, found)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return ErrInvalidRefreshToken
	}
	return s.repo.RevokeRefreshToken(ctx, hashToken(refreshToken), s.now().UTC())
}

func (s *Service) CurrentUser(ctx context.Context, accessToken string) (user.User, error) {
	claims, err := s.verifyAccessToken(accessToken)
	if err != nil {
		return user.User{}, err
	}
	found, err := s.repo.UserByID(ctx, claims.UserID)
	if err != nil || found.Status != user.StatusActive {
		return user.User{}, ErrInvalidAccessToken
	}
	return found, nil
}

func (s *Service) RequireAdmin(ctx context.Context, accessToken string) (user.User, error) {
	found, err := s.CurrentUser(ctx, accessToken)
	if err != nil {
		return user.User{}, err
	}
	if found.Role != user.RoleAdmin {
		return user.User{}, ErrForbidden
	}
	return found, nil
}

func (s *Service) issueSession(ctx context.Context, account user.User) (Session, error) {
	accessToken, expiresAt, err := s.issueAccessToken(account)
	if err != nil {
		return Session{}, err
	}
	refreshToken, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	stored := RefreshToken{
		ID:        newID("rt"),
		UserID:    account.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: now.Add(s.cfg.RefreshTokenTTL),
		CreatedAt: now,
	}
	if err := s.repo.CreateRefreshToken(ctx, stored); err != nil {
		return Session{}, err
	}
	return Session{User: account, AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt}, nil
}

type accessClaims struct {
	UserID string    `json:"sub"`
	Role   user.Role `json:"role"`
	Exp    int64     `json:"exp"`
}

func (s *Service) issueAccessToken(account user.User) (string, time.Time, error) {
	expiresAt := s.now().UTC().Add(s.cfg.AccessTokenTTL)
	claims := accessClaims{UserID: account.ID, Role: account.Role, Exp: expiresAt.Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := sign(encoded, s.cfg.AccessTokenSecret)
	return encoded + "." + signature, expiresAt, nil
}

func (s *Service) verifyAccessToken(token string) (accessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(sign(parts[0], s.cfg.AccessTokenSecret)), []byte(parts[1])) {
		return accessClaims{}, ErrInvalidAccessToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return accessClaims{}, ErrInvalidAccessToken
	}
	var claims accessClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return accessClaims{}, ErrInvalidAccessToken
	}
	if claims.UserID == "" || claims.Exp <= s.now().UTC().Unix() {
		return accessClaims{}, ErrInvalidAccessToken
	}
	return claims, nil
}

func sign(payload string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func newID(prefix string) string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
