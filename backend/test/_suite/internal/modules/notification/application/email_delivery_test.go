package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type emailDeliveryRepositoryStub struct {
	claimed   ClaimedEmailDeliveryDTO
	claim     ClaimNextEmailDeliveryCommand
	started   StartEmailDeliveryCommand
	completed CompleteEmailDeliveryCommand
}

func (stub *emailDeliveryRepositoryStub) ClaimNextEmailDelivery(_ context.Context, command ClaimNextEmailDeliveryCommand) (ClaimedEmailDeliveryDTO, error) {
	stub.claim = command
	stub.claimed.ClaimToken = command.ClaimToken
	stub.claimed.FencingGeneration = 1
	stub.claimed.DispatchKey = strings.Repeat("d", 64)
	stub.claimed.ProviderCapabilities = command.ProviderCapabilities
	return stub.claimed, nil
}

func (stub *emailDeliveryRepositoryStub) StartEmailDelivery(_ context.Context, command StartEmailDeliveryCommand) error {
	stub.started = command
	return nil
}

func (stub *emailDeliveryRepositoryStub) CompleteEmailDelivery(_ context.Context, command CompleteEmailDeliveryCommand) (RecordNotificationDeliveryAttemptResult, error) {
	stub.completed = command
	return RecordNotificationDeliveryAttemptResult{DeliveryAttemptID: 41, AttemptNo: stub.claimed.AttemptCount + 1}, nil
}

type notificationEmailSenderStub struct {
	message NotificationEmailMessageDTO
	err     error
}

func (*notificationEmailSenderStub) Capabilities() NotificationEmailProviderCapabilities {
	return NotificationEmailProviderCapabilities{}
}

func (stub *notificationEmailSenderStub) SendNotificationEmail(_ context.Context, dispatch NotificationEmailDispatchDTO) (string, error) {
	stub.message = dispatch.Message
	return "smtp-41", stub.err
}

func (*notificationEmailSenderStub) LookupNotificationEmail(context.Context, string) (NotificationEmailReceiptDTO, error) {
	return NotificationEmailReceiptDTO{}, errors.New("unsupported")
}

type temporaryMailFailure struct{ temporary bool }

func (failure temporaryMailFailure) Error() string              { return "mail failed" }
func (failure temporaryMailFailure) TemporaryFailure() bool     { return failure.temporary }
func (failure temporaryMailFailure) DeliveryOutcomeKnown() bool { return true }

func TestEmailDeliveryUsesSafeUserNotificationProjection(t *testing.T) {
	now := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	repository := &emailDeliveryRepositoryStub{claimed: validEmailDelivery(now)}
	sender := &notificationEmailSenderStub{}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("a", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.DispatchNext(context.Background())
	if err != nil || !result.Claimed || result.Status != "succeeded" || result.AttemptNo != 1 {
		t.Fatalf("DispatchNext() = %#v / %v", result, err)
	}
	if repository.claim.LeaseDuration != EmailDeliveryLeaseDuration || repository.started.FencingGeneration != 1 ||
		repository.completed.ProviderMessageID != "smtp-41" || repository.completed.Status != "succeeded" {
		t.Fatalf("claim/completion = %#v / %#v", repository.claim, repository.completed)
	}
	if sender.message.Subject != "[HotKey][高] AI 产品 · alert(1)" || strings.Contains(sender.message.HTML, "<script>") ||
		!strings.Contains(sender.message.HTML, "&lt;script&gt;") ||
		!strings.Contains(sender.message.HTML, "AI 产品") || !strings.Contains(sender.message.HTML, "Hacker News") ||
		!strings.Contains(sender.message.HTML, "87.5%") ||
		!strings.Contains(sender.message.HTML, `href="https://news.example.test/item/42"`) ||
		!strings.Contains(sender.message.HTML, `href="https://hotkey.example/dashboard/contents/42"`) {
		t.Fatalf("unsafe or incomplete message = %#v", sender.message)
	}
}

func TestEmailDeliveryRecordsRetryAndAttemptExhaustion(t *testing.T) {
	now := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	for _, fixture := range []struct {
		name         string
		attemptCount int
		failure      error
		wantStatus   string
		wantCode     string
	}{
		{name: "temporary", attemptCount: 0, failure: temporaryMailFailure{temporary: true}, wantStatus: "failed", wantCode: "smtp_temporary"},
		{name: "permanent", attemptCount: 0, failure: temporaryMailFailure{temporary: false}, wantStatus: "permanent_failure", wantCode: "smtp_permanent"},
		{name: "exhausted", attemptCount: 4, failure: temporaryMailFailure{temporary: true}, wantStatus: "permanent_failure", wantCode: "smtp_attempts_exhausted"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			claimed := validEmailDelivery(now)
			claimed.AttemptCount = fixture.attemptCount
			repository := &emailDeliveryRepositoryStub{claimed: claimed}
			service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
				Repository: repository, Sender: &notificationEmailSenderStub{err: fixture.failure},
				NewToken: func() (string, error) { return strings.Repeat("b", 64), nil }, WebOrigin: "http://127.0.0.1:3000",
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.DispatchNext(context.Background())
			if err != nil || result.Status != fixture.wantStatus || repository.completed.ErrorCode != fixture.wantCode {
				t.Fatalf("DispatchNext() = %#v/%v completion=%#v", result, err, repository.completed)
			}
		})
	}
}

func TestEmailDeliveryFailsClosedForInvalidRecipient(t *testing.T) {
	now := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.RecipientEmail = "Name <owner@example.com>"
	repository := &emailDeliveryRepositoryStub{claimed: claimed}
	sender := &notificationEmailSenderStub{}
	service, _ := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("c", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "permanent_failure" || repository.completed.ErrorCode != "invalid_notification_projection" || sender.message.Recipient != "" {
		t.Fatalf("DispatchNext() = %#v/%v completion=%#v message=%#v", result, err, repository.completed, sender.message)
	}
}

func validEmailDelivery(now time.Time) ClaimedEmailDeliveryDTO {
	return ClaimedEmailDeliveryDTO{
		Claimed: true, ClaimToken: strings.Repeat("a", 64), RecipientEmail: "owner@example.com",
		PublishedConfigID: 7, PublishedRevision: 3, AlertEmailEnabled: true,
		MonitorName: "AI 产品", SourceName: "Hacker News", SourceType: "hacker_news",
		RelevanceScore: float64Pointer(87.5), OriginalURL: "https://news.example.test/item/42",
		Notification: UserNotificationDTO{
			ID: 11, Version: 1, OutboxEventID: 10, UserID: 1, MonitorID: 2,
			EventType: "hotspot.discovered", ResourceType: "hotspot", ResourceID: 42, ResourceVersion: 1,
			OccurredAt: now, Title: `<script>alert(1)</script>`, Summary: "安全摘要", ResourceStatus: "high",
			DeepLink: "/dashboard/contents/42", CreatedAt: now,
		},
	}
}

func float64Pointer(value float64) *float64 { return &value }
