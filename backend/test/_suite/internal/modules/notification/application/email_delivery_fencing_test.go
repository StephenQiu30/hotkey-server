package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fencingEmailRepositoryStub struct {
	claimed     ClaimedEmailDeliveryDTO
	started     StartEmailDeliveryCommand
	completed   CompleteEmailDeliveryCommand
	completeErr error
	steps       []string
}

func (stub *fencingEmailRepositoryStub) ClaimNextEmailDelivery(_ context.Context, command ClaimNextEmailDeliveryCommand) (ClaimedEmailDeliveryDTO, error) {
	stub.claimed.ClaimToken = command.ClaimToken
	stub.steps = append(stub.steps, "claim")
	return stub.claimed, nil
}

func (stub *fencingEmailRepositoryStub) StartEmailDelivery(_ context.Context, command StartEmailDeliveryCommand) error {
	stub.started = command
	stub.steps = append(stub.steps, "start")
	return nil
}

func (stub *fencingEmailRepositoryStub) CompleteEmailDelivery(_ context.Context, command CompleteEmailDeliveryCommand) (RecordNotificationDeliveryAttemptResult, error) {
	stub.completed = command
	stub.steps = append(stub.steps, "complete")
	return RecordNotificationDeliveryAttemptResult{DeliveryAttemptID: 41, AttemptNo: stub.claimed.AttemptCount + 1}, stub.completeErr
}

type fencingEmailSenderStub struct {
	capabilities NotificationEmailProviderCapabilities
	dispatch     NotificationEmailDispatchDTO
	receipt      NotificationEmailReceiptDTO
	sendCalls    int
	lookupCalls  int
	sendErr      error
	lookupErr    error
	steps        *[]string
}

func (stub *fencingEmailSenderStub) Capabilities() NotificationEmailProviderCapabilities {
	return stub.capabilities
}

func (stub *fencingEmailSenderStub) SendNotificationEmail(_ context.Context, dispatch NotificationEmailDispatchDTO) (string, error) {
	stub.dispatch = dispatch
	stub.sendCalls++
	*stub.steps = append(*stub.steps, "send")
	return "provider-41", stub.sendErr
}

func (stub *fencingEmailSenderStub) LookupNotificationEmail(_ context.Context, _ string) (NotificationEmailReceiptDTO, error) {
	stub.lookupCalls++
	*stub.steps = append(*stub.steps, "lookup")
	return stub.receipt, stub.lookupErr
}

func TestEmailDeliveryStartsFencedDispatchBeforeCallingProvider(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.FencingGeneration = 4
	claimed.DispatchKey = strings.Repeat("d", 64)
	claimed.ProviderCapabilities = NotificationEmailProviderCapabilities{SupportsIdempotency: true}
	repository := &fencingEmailRepositoryStub{claimed: claimed}
	sender := &fencingEmailSenderStub{capabilities: claimed.ProviderCapabilities, steps: &repository.steps}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("a", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "succeeded" || result.AttemptNo != 1 {
		t.Fatalf("DispatchNext() = %#v / %v", result, err)
	}
	if strings.Join(repository.steps, ",") != "claim,start,send,complete" {
		t.Fatalf("dispatch steps = %v", repository.steps)
	}
	if repository.started.FencingGeneration != 4 || repository.started.DispatchKey != claimed.DispatchKey ||
		sender.dispatch.DispatchKey != claimed.DispatchKey || repository.completed.FencingGeneration != 4 {
		t.Fatalf("start/dispatch/complete = %#v / %#v / %#v", repository.started, sender.dispatch, repository.completed)
	}
}

func TestEmailDeliveryReconcilesProviderReceiptWithoutSendingAgain(t *testing.T) {
	now := time.Date(2026, 8, 28, 2, 0, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.FencingGeneration = 2
	claimed.DispatchKey = strings.Repeat("e", 64)
	claimed.ReconcileRequired = true
	claimed.ProviderCapabilities = NotificationEmailProviderCapabilities{SupportsReceiptLookup: true}
	repository := &fencingEmailRepositoryStub{claimed: claimed}
	sender := &fencingEmailSenderStub{
		capabilities: claimed.ProviderCapabilities,
		receipt:      NotificationEmailReceiptDTO{Found: true, ProviderMessageID: "provider-existing"},
		steps:        &repository.steps,
	}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("b", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "succeeded" || sender.lookupCalls != 1 || sender.sendCalls != 0 ||
		repository.completed.ProviderMessageID != "provider-existing" {
		t.Fatalf("DispatchNext() = %#v/%v sender=%#v completion=%#v", result, err, sender, repository.completed)
	}
}

func TestEmailDeliveryReusesStableDispatchKeyForIdempotentProvider(t *testing.T) {
	now := time.Date(2026, 8, 28, 2, 30, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.FencingGeneration = 3
	claimed.DispatchKey = strings.Repeat("9", 64)
	claimed.ReconcileRequired = true
	claimed.ProviderCapabilities = NotificationEmailProviderCapabilities{SupportsIdempotency: true}
	repository := &fencingEmailRepositoryStub{claimed: claimed}
	sender := &fencingEmailSenderStub{capabilities: claimed.ProviderCapabilities, steps: &repository.steps}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("5", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "succeeded" || sender.sendCalls != 1 || sender.lookupCalls != 0 ||
		sender.dispatch.DispatchKey != claimed.DispatchKey {
		t.Fatalf("DispatchNext() = %#v/%v sender=%#v", result, err, sender)
	}
}

func TestEmailDeliveryStopsWhenProviderReceiptCannotBeConfirmed(t *testing.T) {
	now := time.Date(2026, 8, 28, 2, 45, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.FencingGeneration = 5
	claimed.DispatchKey = strings.Repeat("a", 64)
	claimed.ReconcileRequired = true
	claimed.ProviderCapabilities = NotificationEmailProviderCapabilities{SupportsReceiptLookup: true}
	repository := &fencingEmailRepositoryStub{claimed: claimed}
	sender := &fencingEmailSenderStub{
		capabilities: claimed.ProviderCapabilities,
		lookupErr:    errors.New("provider receipt endpoint unavailable"),
		steps:        &repository.steps,
	}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("b", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "unknown" || sender.lookupCalls != 1 || sender.sendCalls != 0 ||
		repository.completed.Status != "unknown" || repository.completed.ErrorCode != "provider_receipt_unavailable" {
		t.Fatalf("DispatchNext() = %#v/%v sender=%#v completion=%#v", result, err, sender, repository.completed)
	}
}

func TestEmailDeliverySurfacesRecoveredUnknownWithoutCallingProvider(t *testing.T) {
	repository := &fencingEmailRepositoryStub{claimed: ClaimedEmailDeliveryDTO{
		Claimed: true, RecoveredUnknown: true, AttemptCount: 1,
		Notification: UserNotificationDTO{ID: 17},
	}}
	sender := &fencingEmailSenderStub{steps: &repository.steps}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("c", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err != nil || !result.Claimed || result.UserNotificationID != 17 || result.Status != "unknown" ||
		result.AttemptNo != 1 || sender.sendCalls != 0 || repository.started.UserNotificationID != 0 {
		t.Fatalf("DispatchNext() = %#v/%v sender=%#v start=%#v", result, err, sender, repository.started)
	}
}

func TestEmailDeliveryConservativelyStopsOnAmbiguousUnsupportedProviderError(t *testing.T) {
	now := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.FencingGeneration = 1
	claimed.DispatchKey = strings.Repeat("f", 64)
	repository := &fencingEmailRepositoryStub{claimed: claimed}
	sender := &fencingEmailSenderStub{sendErr: errors.New("connection ended without provider receipt"), steps: &repository.steps}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("6", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err != nil || result.Status != "unknown" || repository.completed.Status != "unknown" ||
		repository.completed.ErrorCode != "provider_outcome_unconfirmed" {
		t.Fatalf("DispatchNext() = %#v/%v completion=%#v", result, err, repository.completed)
	}
}

func TestEmailDeliveryLeavesStartedFenceWhenProviderAcceptedButAttemptCommitFails(t *testing.T) {
	now := time.Date(2026, 8, 28, 4, 0, 0, 0, time.UTC)
	claimed := validEmailDelivery(now)
	claimed.FencingGeneration = 1
	claimed.DispatchKey = strings.Repeat("7", 64)
	repository := &fencingEmailRepositoryStub{claimed: claimed, completeErr: errors.New("database unavailable after provider acceptance")}
	sender := &fencingEmailSenderStub{steps: &repository.steps}
	service, err := NewEmailDeliveryService(EmailDeliveryServiceDependencies{
		Repository: repository, Sender: sender,
		NewToken: func() (string, error) { return strings.Repeat("8", 64), nil }, WebOrigin: "https://hotkey.example",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.DispatchNext(context.Background())
	if err == nil || !result.Claimed || repository.started.DispatchKey != claimed.DispatchKey ||
		strings.Join(repository.steps, ",") != "claim,start,send,complete" {
		t.Fatalf("DispatchNext() = %#v/%v steps=%v start=%#v", result, err, repository.steps, repository.started)
	}
}
