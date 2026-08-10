package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type webPushDeliveryRepositoryStub struct {
	claimed        ClaimedWebPushDeliveryDTO
	claimCalls     int
	validateCalls  int
	validateErr    error
	completed      CompleteWebPushDeliveryCommand
	completionCall int
}

func (stub *webPushDeliveryRepositoryStub) ClaimNextWebPushDelivery(_ context.Context, command ClaimNextWebPushDeliveryCommand) (ClaimedWebPushDeliveryDTO, error) {
	stub.claimCalls++
	stub.claimed.ClaimToken = command.ClaimToken
	return stub.claimed, nil
}

func (stub *webPushDeliveryRepositoryStub) ValidateWebPushDeliveryClaim(_ context.Context, _ ValidateWebPushDeliveryClaimQuery) error {
	stub.validateCalls++
	return stub.validateErr
}

func (stub *webPushDeliveryRepositoryStub) CompleteWebPushDelivery(_ context.Context, command CompleteWebPushDeliveryCommand) (RecordNotificationDeliveryAttemptResult, error) {
	stub.completionCall++
	stub.completed = command
	return RecordNotificationDeliveryAttemptResult{DeliveryAttemptID: 71, AttemptNo: stub.claimed.AttemptCount + 1}, nil
}

type pushSubscriptionSecretOpenerStub struct {
	result OpenPushSubscriptionSecretsResult
	err    error
}

func (stub *pushSubscriptionSecretOpenerStub) Open(context.Context, OpenPushSubscriptionSecretsCommand) (OpenPushSubscriptionSecretsResult, error) {
	return stub.result, stub.err
}

type webPushSenderStub struct {
	message WebPushMessageDTO
	result  WebPushSendResult
	err     error
	calls   int
}

func (stub *webPushSenderStub) SendWebPush(_ context.Context, message WebPushMessageDTO) (WebPushSendResult, error) {
	stub.calls++
	stub.message = message
	return stub.result, stub.err
}

func TestWebPushDeliveryUsesMinimalSafePayloadAndRevalidatesPermission(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := &webPushDeliveryRepositoryStub{claimed: validClaimedWebPushDelivery(now)}
	sender := &webPushSenderStub{result: WebPushSendResult{StatusCode: 201, ProviderMessageID: "push-71"}}
	service := mustWebPushDeliveryService(t, repository, sender, now, true)

	result, err := service.DispatchNext(context.Background())
	if err != nil || !result.Claimed || result.Status != "succeeded" || result.AttemptNo != 1 {
		t.Fatalf("DispatchNext() = %#v / %v", result, err)
	}
	if repository.validateCalls != 1 || sender.calls != 1 || repository.completed.ProviderMessageID != "push-71" {
		t.Fatalf("validation/sender/completion = %d/%d/%#v", repository.validateCalls, sender.calls, repository.completed)
	}
	var payload map[string]any
	if err := json.Unmarshal(sender.message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 4 || payload["event_id"] != float64(42) || payload["deep_link"] != "/dashboard/events?event=42" ||
		payload["title"] != "事件发生重要变化" || payload["priority"] != "normal" {
		t.Fatalf("payload = %#v", payload)
	}
	serialized := string(sender.message.Payload)
	for _, forbidden := range []string{"summary", "正文", "object_key", "endpoint", "auth", "p256dh", "token"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("payload leaked %q: %s", forbidden, serialized)
		}
	}
	if sender.message.Topic != "event-42" || sender.message.TTL != 3600 {
		t.Fatalf("message = %#v", sender.message)
	}
}

func TestWebPushDeliveryFailsClosedWhenPermissionChangesAfterClaim(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := &webPushDeliveryRepositoryStub{
		claimed: validClaimedWebPushDelivery(now), validateErr: errors.New("monitor permission revoked"),
	}
	sender := &webPushSenderStub{}
	service := mustWebPushDeliveryService(t, repository, sender, now, true)

	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "permanent_failure" || sender.calls != 0 ||
		repository.completed.ErrorCode != "push_permission_revoked" {
		t.Fatalf("DispatchNext() = %#v/%v sender=%d completion=%#v", result, err, sender.calls, repository.completed)
	}
}

func TestWebPushDeliveryClassifiesGoneTemporaryAndExhaustedResponses(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	fixtures := []struct {
		name           string
		attemptCount   int
		statusCode     int
		sendErr        error
		wantStatus     string
		wantCode       string
		wantExpiration string
	}{
		{name: "gone", statusCode: 410, wantStatus: "permanent_failure", wantCode: "push_subscription_gone", wantExpiration: "push_service_gone"},
		{name: "temporary", statusCode: 503, sendErr: errors.New("unavailable"), wantStatus: "failed", wantCode: "push_temporary"},
		{name: "exhausted", attemptCount: 4, statusCode: 503, sendErr: errors.New("unavailable"), wantStatus: "permanent_failure", wantCode: "push_attempts_exhausted"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			claimed := validClaimedWebPushDelivery(now)
			claimed.AttemptCount = fixture.attemptCount
			repository := &webPushDeliveryRepositoryStub{claimed: claimed}
			sender := &webPushSenderStub{result: WebPushSendResult{StatusCode: fixture.statusCode}, err: fixture.sendErr}
			service := mustWebPushDeliveryService(t, repository, sender, now, true)
			result, err := service.DispatchNext(context.Background())
			if err != nil || result.Status != fixture.wantStatus || repository.completed.ErrorCode != fixture.wantCode ||
				repository.completed.ExpirationReason != fixture.wantExpiration {
				t.Fatalf("DispatchNext() = %#v/%v completion=%#v", result, err, repository.completed)
			}
		})
	}
}

func TestWebPushDeliveryDoesNotClaimWhenRuntimeIsDisabled(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repository := &webPushDeliveryRepositoryStub{claimed: validClaimedWebPushDelivery(now)}
	service := mustWebPushDeliveryService(t, repository, &webPushSenderStub{}, now, false)
	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Claimed || repository.claimCalls != 0 {
		t.Fatalf("DispatchNext() = %#v/%v claims=%d", result, err, repository.claimCalls)
	}
}

func mustWebPushDeliveryService(t *testing.T, repository WebPushDeliveryRepository, sender WebPushSender, now time.Time, enabled bool) *WebPushDeliveryService {
	t.Helper()
	service, err := NewWebPushDeliveryService(WebPushDeliveryServiceDependencies{
		Repository: repository,
		Secrets: &pushSubscriptionSecretOpenerStub{result: OpenPushSubscriptionSecretsResult{
			Endpoint: "https://push.example/subscription/one", P256DH: "p256dh", Auth: "auth",
		}},
		Sender: sender, Enabled: enabled, Clock: func() time.Time { return now },
		NewToken: func() (string, error) { return strings.Repeat("a", 64), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func validClaimedWebPushDelivery(now time.Time) ClaimedWebPushDeliveryDTO {
	return ClaimedWebPushDeliveryDTO{
		Claimed: true, AttemptCount: 0, SubscriptionID: 8, SubscriptionVersion: 1, TTLSeconds: 3600,
		EndpointSHA256: strings.Repeat("b", 64), EndpointCiphertext: []byte(strings.Repeat("e", 48)),
		P256DHCiphertext: []byte(strings.Repeat("p", 48)), AuthCiphertext: []byte(strings.Repeat("a", 48)), EncryptionKeyVersion: 1,
		Notification: UserNotificationDTO{
			ID: 11, Version: 1, OutboxEventID: 10, UserID: 1, MonitorID: 2,
			EventType: "micro_event.updated", ResourceType: "micro_event", ResourceID: 42, ResourceVersion: 1,
			OccurredAt: now, Title: "事件发生重要变化", Summary: "正文摘要不应进入 Push", ResourceStatus: "active",
			DeepLink: "/dashboard/events?event=42", CreatedAt: now,
		},
	}
}
