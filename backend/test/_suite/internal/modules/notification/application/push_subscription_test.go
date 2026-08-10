package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type pushSubscriptionRepositoryStub struct {
	persist PersistPushSubscriptionCommand
	update  UpdatePushSubscriptionCommand
	disable DisablePushSubscriptionCommand
	result  PushSubscriptionDTO
}

func (stub *pushSubscriptionRepositoryStub) PersistPushSubscription(_ context.Context, command PersistPushSubscriptionCommand) (PushSubscriptionDTO, error) {
	stub.persist = command
	return stub.result, nil
}
func (stub *pushSubscriptionRepositoryStub) ListPushSubscriptions(context.Context, ListPushSubscriptionsQuery) (ListPushSubscriptionsResult, error) {
	return ListPushSubscriptionsResult{Items: []PushSubscriptionDTO{stub.result}}, nil
}
func (stub *pushSubscriptionRepositoryStub) UpdatePushSubscription(_ context.Context, command UpdatePushSubscriptionCommand) (PushSubscriptionDTO, error) {
	stub.update = command
	stub.result.Version++
	stub.result.DeviceLabel, stub.result.MonitorIDs = command.DeviceLabel, command.MonitorIDs
	stub.result.UpdatedAt = command.UpdatedAt
	return stub.result, nil
}
func (stub *pushSubscriptionRepositoryStub) DisablePushSubscription(_ context.Context, command DisablePushSubscriptionCommand) (PushSubscriptionDTO, error) {
	stub.disable = command
	stub.result.Version++
	stub.result.Status, stub.result.UpdatedAt = "disabled", command.DisabledAt
	return stub.result, nil
}

type pushSubscriptionCipherStub struct {
	command SealPushSubscriptionSecretsCommand
}

func (stub *pushSubscriptionCipherStub) Seal(_ context.Context, command SealPushSubscriptionSecretsCommand) (SealedPushSubscriptionSecretsDTO, error) {
	stub.command = command
	return SealedPushSubscriptionSecretsDTO{
		EndpointCiphertext: bytes.Repeat([]byte{1}, 48), P256DHCiphertext: bytes.Repeat([]byte{2}, 48),
		AuthCiphertext: bytes.Repeat([]byte{3}, 48), KeyVersion: 1,
	}, nil
}

func TestPushSubscriptionServiceRegistersEncryptedUserOptIn(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	repository := &pushSubscriptionRepositoryStub{result: validPushSubscriptionDTO(now)}
	cipher := &pushSubscriptionCipherStub{}
	service, err := NewPushSubscriptionService(PushSubscriptionServiceDependencies{
		Repository: repository, Cipher: cipher, VAPIDPublicKey: validVAPIDPublicKey(), Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Register(context.Background(), validRegisterPushSubscriptionCommand())
	if err != nil || result.ID != 4 || !service.Capability().Available {
		t.Fatalf("Register() = %#v / %v", result, err)
	}
	if repository.persist.EndpointSHA256 == "" || repository.persist.CommandFingerprint == "" ||
		len(repository.persist.EndpointCiphertext) == 0 || cipher.command.Endpoint == "" {
		t.Fatalf("persist/cipher commands = %#v / %#v", repository.persist, cipher.command)
	}
	if string(repository.persist.EndpointCiphertext) == cipher.command.Endpoint || repository.persist.CreatedAt != now {
		t.Fatal("plaintext endpoint escaped the encrypted persistence boundary")
	}
}

func TestPushSubscriptionServiceRequiresCapabilityAndStrongVersion(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	repository := &pushSubscriptionRepositoryStub{result: validPushSubscriptionDTO(now)}
	service, err := NewPushSubscriptionService(PushSubscriptionServiceDependencies{Repository: repository, Cipher: &pushSubscriptionCipherStub{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Register(context.Background(), validRegisterPushSubscriptionCommand()); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := service.Update(context.Background(), UpdatePushSubscriptionCommand{UserID: 1, SubscriptionID: 4}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("Update() error = %v", err)
	}
}

func validRegisterPushSubscriptionCommand() RegisterPushSubscriptionCommand {
	p256dh := append([]byte{4}, bytes.Repeat([]byte{5}, 64)...)
	return RegisterPushSubscriptionCommand{
		UserID: 1, Endpoint: "https://push.example/subscription/one",
		P256DH: base64.RawURLEncoding.EncodeToString(p256dh), Auth: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 16)),
		DeviceLabel: "iPhone", Timezone: "Asia/Shanghai", TTLSeconds: 3600, MonitorIDs: []int64{9, 3},
		IdempotencyKey: "push-register-test-1",
	}
}

func validPushSubscriptionDTO(now time.Time) PushSubscriptionDTO {
	return PushSubscriptionDTO{
		ID: 4, Version: 1, UserID: 1, DeviceLabel: "iPhone", Timezone: "Asia/Shanghai",
		TTLSeconds: 3600, Status: "active", MonitorIDs: []int64{3, 9}, CreatedAt: now, UpdatedAt: now,
	}
}

func validVAPIDPublicKey() string {
	return base64.RawURLEncoding.EncodeToString(append([]byte{4}, bytes.Repeat([]byte{9}, 64)...))
}
