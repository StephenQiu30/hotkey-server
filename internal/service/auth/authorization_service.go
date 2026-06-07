package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/authorization"
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
)

type AuthorizationRepository interface {
	CreateAuthorization(ctx context.Context, az authorization.Authorization) (authorization.Authorization, error)
	AuthorizationByID(ctx context.Context, id string) (authorization.Authorization, error)
	AuthorizationsByUserID(ctx context.Context, userID string) ([]authorization.Authorization, error)
	UpdateAuthorizationStatus(ctx context.Context, id string, status authorization.Status, now time.Time) error
	TouchAuthorization(ctx context.Context, id string, now time.Time) error
	RevokeAllByUserID(ctx context.Context, userID string, now time.Time) error
	DeleteAuthorizationsByUserID(ctx context.Context, userID string) error
}

type ConnectInput struct {
	UserID         string
	Platform       authorization.Platform
	PlatformUserID string
	DisplayName    string
	AccessToken    string
	RefreshToken   string
	ExpiresAt      *time.Time
}

type AuthorizationService struct {
	authRepo    Repository
	azRepo      AuthorizationRepository
	encryptor   crypto.Encryptor
	transactor  Transactor
	now         func() time.Time
}

type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

var ErrNilEncryptor = errors.New("encryptor must not be nil")

func NewAuthorizationService(authRepo Repository, azRepo AuthorizationRepository, encryptor crypto.Encryptor, now func() time.Time) (*AuthorizationService, error) {
	if encryptor == nil {
		return nil, ErrNilEncryptor
	}
	if now == nil {
		now = time.Now
	}
	if azRepo == nil {
		azRepo = NewMemoryAuthorizationRepository()
	}
	return &AuthorizationService{
		authRepo:  authRepo,
		azRepo:    azRepo,
		encryptor: encryptor,
		now:       now,
	}, nil
}

func (s *AuthorizationService) WithTransactor(t Transactor) *AuthorizationService {
	s.transactor = t
	return s
}

func (s *AuthorizationService) Connect(ctx context.Context, input ConnectInput) (authorization.Authorization, error) {
	return s.ConnectWithExpiry(ctx, input, input.ExpiresAt)
}

func (s *AuthorizationService) ConnectWithExpiry(ctx context.Context, input ConnectInput, expiresAt *time.Time) (authorization.Authorization, error) {
	if !authorization.ValidPlatform(input.Platform) {
		return authorization.Authorization{}, authorization.ErrInvalidPlatform
	}

	// Verify user exists
	if _, err := s.authRepo.UserByID(ctx, input.UserID); err != nil {
		return authorization.Authorization{}, fmt.Errorf("user not found: %w", err)
	}

	// Encrypt access token
	encToken, err := s.encryptor.Encrypt(input.AccessToken)
	if err != nil {
		return authorization.Authorization{}, fmt.Errorf("encrypt token: %w", err)
	}

	var encRefresh string
	if input.RefreshToken != "" {
		encRefresh, err = s.encryptor.Encrypt(input.RefreshToken)
		if err != nil {
			return authorization.Authorization{}, fmt.Errorf("encrypt refresh token: %w", err)
		}
	}

	now := s.now().UTC()
	az := authorization.Authorization{
		ID:              newAuthID("az"),
		UserID:          input.UserID,
		Platform:        input.Platform,
		PlatformUserID:  input.PlatformUserID,
		DisplayName:     input.DisplayName,
		AccessTokenEnc:  encToken,
		RefreshTokenEnc: encRefresh,
		Status:          authorization.StatusConnected,
		ConnectedAt:     now,
		LastCheckedAt:   now,
		ExpiresAt:       expiresAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	created, err := s.azRepo.CreateAuthorization(ctx, az)
	if err != nil {
		return authorization.Authorization{}, err
	}
	return created, nil
}

func (s *AuthorizationService) Disconnect(ctx context.Context, userID string, azID string) error {
	az, err := s.azRepo.AuthorizationByID(ctx, azID)
	if err != nil {
		if errors.Is(err, authorization.ErrNotFound) {
			return authorization.ErrNotFound
		}
		return err
	}
	if az.UserID != userID {
		return authorization.ErrNotFound
	}
	if az.IsRevoked() {
		return authorization.ErrAlreadyRevoked
	}
	return s.azRepo.UpdateAuthorizationStatus(ctx, azID, authorization.StatusRevoked, s.now().UTC())
}

func (s *AuthorizationService) HealthCheck(ctx context.Context, userID string, azID string) (authorization.Authorization, error) {
	az, err := s.azRepo.AuthorizationByID(ctx, azID)
	if err != nil {
		if errors.Is(err, authorization.ErrNotFound) {
			return authorization.Authorization{}, authorization.ErrNotFound
		}
		return authorization.Authorization{}, err
	}

	// Verify ownership before mutating
	if az.UserID != userID {
		return authorization.Authorization{}, authorization.ErrNotFound
	}

	if az.IsRevoked() {
		return az, nil
	}

	now := s.now().UTC()
	if az.IsExpired(now) {
		if err := s.azRepo.UpdateAuthorizationStatus(ctx, azID, authorization.StatusExpired, now); err != nil {
			return authorization.Authorization{}, err
		}
		az.Status = authorization.StatusExpired
		az.UpdatedAt = now
		return az, nil
	}

	// Persist last checked time
	if err := s.azRepo.TouchAuthorization(ctx, azID, now); err != nil {
		return authorization.Authorization{}, err
	}
	az.LastCheckedAt = now
	az.UpdatedAt = now
	return az, nil
}

func (s *AuthorizationService) ListByUser(ctx context.Context, userID string) ([]authorization.Authorization, error) {
	return s.azRepo.AuthorizationsByUserID(ctx, userID)
}

func (s *AuthorizationService) DeleteAccount(ctx context.Context, userID string) error {
	now := s.now().UTC()

	fn := func(ctx context.Context) error {
		// Revoke all authorizations
		if err := s.azRepo.RevokeAllByUserID(ctx, userID, now); err != nil {
			return fmt.Errorf("revoke authorizations: %w", err)
		}

		// Delete authorizations
		if err := s.azRepo.DeleteAuthorizationsByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete authorizations: %w", err)
		}

		// Delete all refresh tokens for the user
		if err := s.authRepo.DeleteRefreshTokensByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete refresh tokens: %w", err)
		}

		// Delete the user
		if err := s.authRepo.DeleteUser(ctx, userID); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}

		return nil
	}

	if s.transactor != nil {
		return s.transactor.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}

func newAuthID(prefix string) string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
