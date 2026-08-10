package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	MaximumWebPushAttempts       = 5
	WebPushDeliveryLeaseDuration = 2 * time.Minute
)

func WebPushDeliveryTarget(subscriptionID int64) string {
	return fmt.Sprintf("push_subscription:%d", subscriptionID)
}

type OpenPushSubscriptionSecretsCommand struct {
	EndpointSHA256     string
	EndpointCiphertext []byte
	P256DHCiphertext   []byte
	AuthCiphertext     []byte
	KeyVersion         int
}

type OpenPushSubscriptionSecretsResult struct {
	Endpoint string
	P256DH   string
	Auth     string
}

type PushSubscriptionSecretOpener interface {
	Open(context.Context, OpenPushSubscriptionSecretsCommand) (OpenPushSubscriptionSecretsResult, error)
}

type ClaimNextWebPushDeliveryCommand struct {
	ClaimToken string
	ClaimedAt  time.Time
	LeaseUntil time.Time
}

type ClaimedWebPushDeliveryDTO struct {
	Claimed              bool
	ClaimToken           string
	AttemptCount         int
	SubscriptionID       int64
	SubscriptionVersion  int64
	TTLSeconds           int
	EndpointSHA256       string
	EndpointCiphertext   []byte
	P256DHCiphertext     []byte
	AuthCiphertext       []byte
	EncryptionKeyVersion int
	Notification         UserNotificationDTO
}

type CompleteWebPushDeliveryCommand struct {
	UserNotificationID int64
	UserID             int64
	SubscriptionID     int64
	ClaimToken         string
	Status             string
	ProviderMessageID  string
	ResponseCode       *int
	ErrorCode          string
	ExpirationReason   string
	AttemptedAt        time.Time
}

type WebPushDeliveryRepository interface {
	ClaimNextWebPushDelivery(context.Context, ClaimNextWebPushDeliveryCommand) (ClaimedWebPushDeliveryDTO, error)
	ValidateWebPushDeliveryClaim(context.Context, ValidateWebPushDeliveryClaimQuery) error
	CompleteWebPushDelivery(context.Context, CompleteWebPushDeliveryCommand) (RecordNotificationDeliveryAttemptResult, error)
}

type ValidateWebPushDeliveryClaimQuery struct {
	UserNotificationID int64
	SubscriptionID     int64
	ClaimToken         string
	ValidatedAt        time.Time
}

type WebPushMessageDTO struct {
	Endpoint string
	P256DH   string
	Auth     string
	Payload  []byte
	TTL      int
	Topic    string
}

type WebPushSendResult struct {
	StatusCode        int
	ProviderMessageID string
}

type WebPushSender interface {
	SendWebPush(context.Context, WebPushMessageDTO) (WebPushSendResult, error)
}

type WebPushDeliveryServiceDependencies struct {
	Repository WebPushDeliveryRepository
	Secrets    PushSubscriptionSecretOpener
	Sender     WebPushSender
	Enabled    bool
	Clock      func() time.Time
	NewToken   func() (string, error)
}

type WebPushDeliveryService struct {
	repository WebPushDeliveryRepository
	secrets    PushSubscriptionSecretOpener
	sender     WebPushSender
	clock      func() time.Time
	newToken   func() (string, error)
	enabled    bool
}

type DispatchWebPushDeliveryResult struct {
	Claimed            bool
	UserNotificationID int64
	SubscriptionID     int64
	Status             string
	AttemptNo          int
}

type webPushPayload struct {
	Title    string `json:"title"`
	EventID  int64  `json:"event_id"`
	DeepLink string `json:"deep_link"`
	Priority string `json:"priority"`
}

func NewWebPushDeliveryService(dependencies WebPushDeliveryServiceDependencies) (*WebPushDeliveryService, error) {
	if dependencies.Repository == nil || dependencies.Secrets == nil || dependencies.Sender == nil {
		return nil, fmt.Errorf("Web Push delivery dependencies are required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = func() time.Time { return time.Now().UTC() }
	}
	if dependencies.NewToken == nil {
		dependencies.NewToken = newWebPushClaimToken
	}
	return &WebPushDeliveryService{
		repository: dependencies.Repository, secrets: dependencies.Secrets, sender: dependencies.Sender,
		clock: dependencies.Clock, newToken: dependencies.NewToken,
		enabled: dependencies.Enabled,
	}, nil
}

func (service *WebPushDeliveryService) DispatchNext(ctx context.Context) (DispatchWebPushDeliveryResult, error) {
	if service == nil {
		return DispatchWebPushDeliveryResult{}, sharedrepository.ErrUnavailable
	}
	if !service.enabled {
		return DispatchWebPushDeliveryResult{}, nil
	}
	now := service.clock().UTC()
	token, err := service.newToken()
	if err != nil {
		return DispatchWebPushDeliveryResult{}, fmt.Errorf("create Web Push claim: %w", err)
	}
	claimed, err := service.repository.ClaimNextWebPushDelivery(ctx, ClaimNextWebPushDeliveryCommand{
		ClaimToken: token, ClaimedAt: now, LeaseUntil: now.Add(WebPushDeliveryLeaseDuration),
	})
	if err != nil {
		return DispatchWebPushDeliveryResult{}, err
	}
	if !claimed.Claimed {
		return DispatchWebPushDeliveryResult{}, nil
	}
	result := DispatchWebPushDeliveryResult{
		Claimed: true, UserNotificationID: claimed.Notification.ID, SubscriptionID: claimed.SubscriptionID,
	}
	status, errorCode, expirationReason := "succeeded", "", ""
	var responseCode *int
	providerMessageID := WebPushDeliveryTarget(claimed.SubscriptionID)
	remainingTTL := int(claimed.Notification.OccurredAt.Add(time.Duration(claimed.TTLSeconds) * time.Second).Sub(now).Seconds())
	if remainingTTL <= 0 {
		status, errorCode = "permanent_failure", "push_ttl_expired"
	} else if err := validateClaimedWebPushDelivery(claimed, token); err != nil {
		status, errorCode, expirationReason = "permanent_failure", "invalid_push_subscription", "subscription_integrity"
	} else {
		opened, openErr := service.secrets.Open(ctx, OpenPushSubscriptionSecretsCommand{
			EndpointSHA256: claimed.EndpointSHA256, EndpointCiphertext: claimed.EndpointCiphertext,
			P256DHCiphertext: claimed.P256DHCiphertext, AuthCiphertext: claimed.AuthCiphertext,
			KeyVersion: claimed.EncryptionKeyVersion,
		})
		if openErr != nil {
			status, errorCode, expirationReason = "permanent_failure", "push_secret_unavailable", "subscription_integrity"
		} else {
			if err := service.repository.ValidateWebPushDeliveryClaim(ctx, ValidateWebPushDeliveryClaimQuery{
				UserNotificationID: claimed.Notification.ID, SubscriptionID: claimed.SubscriptionID,
				ClaimToken: token, ValidatedAt: service.clock().UTC(),
			}); err != nil {
				status, errorCode = "permanent_failure", "push_permission_revoked"
				goto complete
			}
			payload, payloadErr := json.Marshal(webPushPayload{
				Title: cleanNotificationText(claimed.Notification.Title), EventID: claimed.Notification.ResourceID,
				DeepLink: claimed.Notification.DeepLink, Priority: "normal",
			})
			if payloadErr != nil {
				return DispatchWebPushDeliveryResult{}, fmt.Errorf("encode Web Push payload: %w", payloadErr)
			}
			sent, sendErr := service.sender.SendWebPush(ctx, WebPushMessageDTO{
				Endpoint: opened.Endpoint, P256DH: opened.P256DH, Auth: opened.Auth, Payload: payload,
				TTL: remainingTTL, Topic: fmt.Sprintf("event-%d", claimed.Notification.ResourceID),
			})
			if sent.ProviderMessageID != "" {
				providerMessageID = sent.ProviderMessageID
			}
			if sent.StatusCode > 0 {
				code := sent.StatusCode
				responseCode = &code
			}
			status, errorCode, expirationReason = classifyWebPushOutcome(sent.StatusCode, sendErr, claimed.AttemptCount+1)
		}
	}

complete:
	completed, err := service.repository.CompleteWebPushDelivery(ctx, CompleteWebPushDeliveryCommand{
		UserNotificationID: claimed.Notification.ID, UserID: claimed.Notification.UserID,
		SubscriptionID: claimed.SubscriptionID, ClaimToken: token, Status: status,
		ProviderMessageID: providerMessageID, ResponseCode: responseCode, ErrorCode: errorCode,
		ExpirationReason: expirationReason, AttemptedAt: service.clock().UTC(),
	})
	if err != nil {
		return DispatchWebPushDeliveryResult{}, err
	}
	result.Status, result.AttemptNo = status, completed.AttemptNo
	return result, nil
}

func validateClaimedWebPushDelivery(claimed ClaimedWebPushDeliveryDTO, expectedToken string) error {
	if !claimed.Claimed || claimed.ClaimToken != expectedToken || claimed.SubscriptionID <= 0 ||
		claimed.SubscriptionVersion <= 0 || claimed.AttemptCount < 0 || claimed.AttemptCount >= MaximumWebPushAttempts ||
		claimed.TTLSeconds < 60 || claimed.TTLSeconds > 86400 || len(claimed.EndpointSHA256) != 64 ||
		len(claimed.EndpointCiphertext) < 32 || len(claimed.P256DHCiphertext) < 32 || len(claimed.AuthCiphertext) < 32 ||
		claimed.EncryptionKeyVersion <= 0 {
		return fmt.Errorf("claimed Web Push delivery is invalid")
	}
	return ValidateUserNotificationDTO(claimed.Notification)
}

func classifyWebPushOutcome(statusCode int, sendErr error, attemptNo int) (string, string, string) {
	if sendErr == nil && statusCode >= 200 && statusCode < 300 {
		return "succeeded", "", ""
	}
	if statusCode == 404 || statusCode == 410 {
		return "permanent_failure", "push_subscription_gone", "push_service_gone"
	}
	if statusCode == 400 || statusCode == 401 || statusCode == 403 {
		return "permanent_failure", "push_request_rejected", ""
	}
	if attemptNo >= MaximumWebPushAttempts {
		return "permanent_failure", "push_attempts_exhausted", ""
	}
	return "failed", "push_temporary", ""
}

func newWebPushClaimToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
