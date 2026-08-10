package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type PushCapabilityDTO struct {
	Available      bool
	VAPIDPublicKey string
}

type PushSubscriptionDTO struct {
	ID               int64
	Version          int64
	UserID           int64
	DeviceLabel      string
	Timezone         string
	QuietStart       *string
	QuietEnd         *string
	TTLSeconds       int
	Status           string
	ExpirationReason string
	MonitorIDs       []int64
	LastSuccessAt    *time.Time
	LastFailureAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type SealedPushSubscriptionSecretsDTO struct {
	EndpointCiphertext []byte
	P256DHCiphertext   []byte
	AuthCiphertext     []byte
	KeyVersion         int
}

type SealPushSubscriptionSecretsCommand struct {
	EndpointSHA256 string
	Endpoint       string
	P256DH         string
	Auth           string
}

type PushSubscriptionCipher interface {
	Seal(context.Context, SealPushSubscriptionSecretsCommand) (SealedPushSubscriptionSecretsDTO, error)
}

type PersistPushSubscriptionCommand struct {
	UserID               int64
	EndpointSHA256       string
	EndpointCiphertext   []byte
	P256DHCiphertext     []byte
	AuthCiphertext       []byte
	EncryptionKeyVersion int
	DeviceLabel          string
	Timezone             string
	QuietStart           *string
	QuietEnd             *string
	TTLSeconds           int
	MonitorIDs           []int64
	IdempotencyKey       string
	CommandFingerprint   string
	CreatedAt            time.Time
}

type ListPushSubscriptionsQuery struct {
	UserID int64
}

type ListPushSubscriptionsResult struct {
	Items []PushSubscriptionDTO
}

type UpdatePushSubscriptionCommand struct {
	UserID          int64
	SubscriptionID  int64
	ExpectedVersion int64
	DeviceLabel     string
	Timezone        string
	QuietStart      *string
	QuietEnd        *string
	TTLSeconds      int
	MonitorIDs      []int64
	UpdatedAt       time.Time
}

type DisablePushSubscriptionCommand struct {
	UserID          int64
	SubscriptionID  int64
	ExpectedVersion int64
	DisabledAt      time.Time
}

type PushSubscriptionRepository interface {
	PersistPushSubscription(context.Context, PersistPushSubscriptionCommand) (PushSubscriptionDTO, error)
	ListPushSubscriptions(context.Context, ListPushSubscriptionsQuery) (ListPushSubscriptionsResult, error)
	UpdatePushSubscription(context.Context, UpdatePushSubscriptionCommand) (PushSubscriptionDTO, error)
	DisablePushSubscription(context.Context, DisablePushSubscriptionCommand) (PushSubscriptionDTO, error)
}

type PushSubscriptionServiceDependencies struct {
	Repository     PushSubscriptionRepository
	Cipher         PushSubscriptionCipher
	VAPIDPublicKey string
	Clock          func() time.Time
}

type PushSubscriptionService struct {
	repository     PushSubscriptionRepository
	cipher         PushSubscriptionCipher
	vapidPublicKey string
	clock          func() time.Time
}

type RegisterPushSubscriptionCommand struct {
	UserID         int64
	Endpoint       string
	P256DH         string
	Auth           string
	DeviceLabel    string
	Timezone       string
	QuietStart     *string
	QuietEnd       *string
	TTLSeconds     int
	MonitorIDs     []int64
	IdempotencyKey string
}

func NewPushSubscriptionService(dependencies PushSubscriptionServiceDependencies) (*PushSubscriptionService, error) {
	if dependencies.Repository == nil || dependencies.Cipher == nil {
		return nil, fmt.Errorf("push subscription dependencies are required")
	}
	publicKey := strings.TrimSpace(dependencies.VAPIDPublicKey)
	if publicKey != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(publicKey)
		if err != nil || len(decoded) != 65 || decoded[0] != 4 {
			return nil, fmt.Errorf("VAPID public key is invalid")
		}
	}
	if dependencies.Clock == nil {
		dependencies.Clock = func() time.Time { return time.Now().UTC() }
	}
	return &PushSubscriptionService{
		repository: dependencies.Repository, cipher: dependencies.Cipher,
		vapidPublicKey: publicKey, clock: dependencies.Clock,
	}, nil
}

func (service *PushSubscriptionService) Capability() PushCapabilityDTO {
	if service == nil || service.vapidPublicKey == "" {
		return PushCapabilityDTO{}
	}
	return PushCapabilityDTO{Available: true, VAPIDPublicKey: service.vapidPublicKey}
}

func (service *PushSubscriptionService) Register(ctx context.Context, command RegisterPushSubscriptionCommand) (PushSubscriptionDTO, error) {
	if service == nil || !service.Capability().Available {
		return PushSubscriptionDTO{}, sharedrepository.ErrUnavailable
	}
	subscription, err := domain.NormalizePushSubscription(domain.PushSubscription{
		UserID: command.UserID, Endpoint: command.Endpoint, P256DH: command.P256DH, Auth: command.Auth,
		DeviceLabel: command.DeviceLabel, Timezone: command.Timezone, QuietStart: command.QuietStart,
		QuietEnd: command.QuietEnd, TTLSeconds: command.TTLSeconds, MonitorIDs: command.MonitorIDs,
		Status: domain.PushSubscriptionActive, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return PushSubscriptionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	endpointSHA256 := domain.PushEndpointSHA256(subscription.Endpoint)
	sealed, err := service.cipher.Seal(ctx, SealPushSubscriptionSecretsCommand{
		EndpointSHA256: endpointSHA256, Endpoint: subscription.Endpoint, P256DH: subscription.P256DH, Auth: subscription.Auth,
	})
	if err != nil {
		return PushSubscriptionDTO{}, err
	}
	if err := validateSealedPushSubscriptionSecrets(sealed); err != nil {
		return PushSubscriptionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	now := service.clock().UTC()
	result, err := service.repository.PersistPushSubscription(ctx, PersistPushSubscriptionCommand{
		UserID: subscription.UserID, EndpointSHA256: endpointSHA256,
		EndpointCiphertext: append([]byte(nil), sealed.EndpointCiphertext...),
		P256DHCiphertext:   append([]byte(nil), sealed.P256DHCiphertext...), AuthCiphertext: append([]byte(nil), sealed.AuthCiphertext...),
		EncryptionKeyVersion: sealed.KeyVersion, DeviceLabel: subscription.DeviceLabel, Timezone: subscription.Timezone,
		QuietStart: subscription.QuietStart, QuietEnd: subscription.QuietEnd, TTLSeconds: subscription.TTLSeconds,
		MonitorIDs: append([]int64(nil), subscription.MonitorIDs...), IdempotencyKey: subscription.IdempotencyKey,
		CommandFingerprint: domain.PushSubscriptionFingerprint(subscription), CreatedAt: now,
	})
	if err != nil {
		return PushSubscriptionDTO{}, err
	}
	if err := ValidatePushSubscriptionDTO(result, command.UserID); err != nil {
		return PushSubscriptionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return result, nil
}

func (service *PushSubscriptionService) List(ctx context.Context, query ListPushSubscriptionsQuery) (ListPushSubscriptionsResult, error) {
	if service == nil || query.UserID <= 0 {
		return ListPushSubscriptionsResult{}, sharedrepository.ErrInvalidInput
	}
	result, err := service.repository.ListPushSubscriptions(ctx, query)
	if err != nil {
		return ListPushSubscriptionsResult{}, err
	}
	for _, item := range result.Items {
		if err := ValidatePushSubscriptionDTO(item, query.UserID); err != nil {
			return ListPushSubscriptionsResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
	}
	return result, nil
}

func (service *PushSubscriptionService) Update(ctx context.Context, command UpdatePushSubscriptionCommand) (PushSubscriptionDTO, error) {
	if service == nil || command.UserID <= 0 || command.SubscriptionID <= 0 || command.ExpectedVersion <= 0 {
		return PushSubscriptionDTO{}, sharedrepository.ErrInvalidInput
	}
	preferences, err := domain.NormalizePushSubscriptionPreferences(domain.PushSubscriptionPreferences{
		DeviceLabel: command.DeviceLabel, Timezone: command.Timezone, QuietStart: command.QuietStart,
		QuietEnd: command.QuietEnd, TTLSeconds: command.TTLSeconds, MonitorIDs: command.MonitorIDs,
	})
	if err != nil {
		return PushSubscriptionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	command.DeviceLabel, command.Timezone = preferences.DeviceLabel, preferences.Timezone
	command.QuietStart, command.QuietEnd = preferences.QuietStart, preferences.QuietEnd
	command.TTLSeconds, command.MonitorIDs = preferences.TTLSeconds, preferences.MonitorIDs
	command.UpdatedAt = service.clock().UTC()
	result, err := service.repository.UpdatePushSubscription(ctx, command)
	if err != nil {
		return PushSubscriptionDTO{}, err
	}
	if err := ValidatePushSubscriptionDTO(result, command.UserID); err != nil {
		return PushSubscriptionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return result, nil
}

func (service *PushSubscriptionService) Disable(ctx context.Context, command DisablePushSubscriptionCommand) (PushSubscriptionDTO, error) {
	if service == nil || command.UserID <= 0 || command.SubscriptionID <= 0 || command.ExpectedVersion <= 0 {
		return PushSubscriptionDTO{}, sharedrepository.ErrInvalidInput
	}
	command.DisabledAt = service.clock().UTC()
	result, err := service.repository.DisablePushSubscription(ctx, command)
	if err != nil {
		return PushSubscriptionDTO{}, err
	}
	if err := ValidatePushSubscriptionDTO(result, command.UserID); err != nil || result.Status != string(domain.PushSubscriptionDisabled) {
		return PushSubscriptionDTO{}, sharedrepository.ErrConstraint
	}
	return result, nil
}

func ValidatePushSubscriptionDTO(subscription PushSubscriptionDTO, expectedUserID int64) error {
	if subscription.ID <= 0 || subscription.Version <= 0 || subscription.UserID != expectedUserID ||
		subscription.CreatedAt.IsZero() || subscription.UpdatedAt.IsZero() {
		return fmt.Errorf("push subscription projection identity is invalid")
	}
	preferences, err := domain.NormalizePushSubscriptionPreferences(domain.PushSubscriptionPreferences{
		DeviceLabel: subscription.DeviceLabel, Timezone: subscription.Timezone, QuietStart: subscription.QuietStart,
		QuietEnd: subscription.QuietEnd, TTLSeconds: subscription.TTLSeconds, MonitorIDs: subscription.MonitorIDs,
	})
	if err != nil || preferences.DeviceLabel != subscription.DeviceLabel || preferences.Timezone != subscription.Timezone {
		return fmt.Errorf("push subscription projection preferences are invalid")
	}
	status := domain.PushSubscriptionStatus(subscription.Status)
	if !status.Valid() || status == domain.PushSubscriptionExpired && strings.TrimSpace(subscription.ExpirationReason) == "" ||
		status != domain.PushSubscriptionExpired && subscription.ExpirationReason != "" {
		return fmt.Errorf("push subscription projection status is invalid")
	}
	return nil
}

func validateSealedPushSubscriptionSecrets(sealed SealedPushSubscriptionSecretsDTO) error {
	if sealed.KeyVersion <= 0 || len(sealed.EndpointCiphertext) < 32 || len(sealed.EndpointCiphertext) > 8192 ||
		len(sealed.P256DHCiphertext) < 32 || len(sealed.P256DHCiphertext) > 1024 ||
		len(sealed.AuthCiphertext) < 32 || len(sealed.AuthCiphertext) > 512 {
		return fmt.Errorf("sealed push subscription secrets are invalid")
	}
	return nil
}
