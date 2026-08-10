//go:build integration

package postgres

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestPushSubscriptionRepositoryEncryptsAtRestReplaysAndEnforcesOwnerCAS(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	now := time.Now().UTC().Truncate(time.Microsecond)
	userID, monitorID, _ := insertEmailNotificationFixture(t, runtime, now)

	command := pushSubscriptionPersistenceFixture(userID, monitorID, 1, now)
	created, err := repository.PersistPushSubscription(ctx, command)
	if err != nil || created.ID <= 0 || created.Version != 1 || len(created.MonitorIDs) != 1 || created.MonitorIDs[0] != monitorID {
		t.Fatalf("PersistPushSubscription() = %#v / %v", created, err)
	}
	replayed, err := repository.PersistPushSubscription(ctx, command)
	if err != nil || replayed.ID != created.ID || replayed.Version != created.Version {
		t.Fatalf("replay = %#v / %v", replayed, err)
	}
	refreshedBrowserReplay := command
	refreshedBrowserReplay.IdempotencyKey = "push-subscription-browser-refresh"
	refreshed, err := repository.PersistPushSubscription(ctx, refreshedBrowserReplay)
	if err != nil || refreshed.ID != created.ID || refreshed.Version != created.Version {
		t.Fatalf("browser refresh replay = %#v / %v", refreshed, err)
	}
	conflict := command
	conflict.CommandFingerprint = strings.Repeat("f", 64)
	if _, err := repository.PersistPushSubscription(ctx, conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	var endpointCiphertext, p256dhCiphertext, authCiphertext []byte
	if err := runtime.SQL.QueryRow(`SELECT endpoint_ciphertext,p256dh_ciphertext,auth_ciphertext
FROM web_push_subscriptions WHERE id=$1`, created.ID).Scan(&endpointCiphertext, &p256dhCiphertext, &authCiphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(endpointCiphertext, []byte("https://push.example/subscription/1")) ||
		bytes.Contains(endpointCiphertext, []byte("push.example")) || bytes.Contains(p256dhCiphertext, []byte("p256dh")) ||
		bytes.Contains(authCiphertext, []byte("auth")) {
		t.Fatal("push subscription secrets were persisted in plaintext")
	}

	otherUserID := insertPushUser(t, runtime, now.Add(time.Second))
	other, err := repository.ListPushSubscriptions(ctx, application.ListPushSubscriptionsQuery{UserID: otherUserID})
	if err != nil || len(other.Items) != 0 {
		t.Fatalf("other user list = %#v / %v", other, err)
	}
	quietStart, quietEnd := "22:00", "07:00"
	updated, err := repository.UpdatePushSubscription(ctx, application.UpdatePushSubscriptionCommand{
		UserID: userID, SubscriptionID: created.ID, ExpectedVersion: 1, DeviceLabel: "工作手机",
		Timezone: "Asia/Shanghai", QuietStart: &quietStart, QuietEnd: &quietEnd, TTLSeconds: 7200,
		MonitorIDs: []int64{monitorID}, UpdatedAt: now.Add(time.Second),
	})
	if err != nil || updated.Version != 2 || updated.DeviceLabel != "工作手机" || updated.QuietStart == nil {
		t.Fatalf("UpdatePushSubscription() = %#v / %v", updated, err)
	}
	if _, err := repository.UpdatePushSubscription(ctx, application.UpdatePushSubscriptionCommand{
		UserID: userID, SubscriptionID: created.ID, ExpectedVersion: 1, DeviceLabel: "stale",
		Timezone: "UTC", TTLSeconds: 3600, MonitorIDs: []int64{monitorID}, UpdatedAt: now.Add(2 * time.Second),
	}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	disabled, err := repository.DisablePushSubscription(ctx, application.DisablePushSubscriptionCommand{
		UserID: userID, SubscriptionID: created.ID, ExpectedVersion: 2, DisabledAt: now.Add(3 * time.Second),
	})
	if err != nil || disabled.Status != "disabled" || disabled.Version != 3 {
		t.Fatalf("DisablePushSubscription() = %#v / %v", disabled, err)
	}
}

func TestPushSubscriptionRepositoryRebindsOwnedBrowserEndpointAfterKeyRotationOrDisable(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	now := time.Now().UTC().Truncate(time.Microsecond)
	userID, monitorID, _ := insertEmailNotificationFixture(t, runtime, now)
	command := pushSubscriptionPersistenceFixture(userID, monitorID, 2, now)
	created, err := repository.PersistPushSubscription(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := repository.DisablePushSubscription(ctx, application.DisablePushSubscriptionCommand{
		UserID: userID, SubscriptionID: created.ID, ExpectedVersion: created.Version, DisabledAt: now.Add(time.Second),
	})
	if err != nil || disabled.Status != "disabled" {
		t.Fatalf("disable = %#v / %v", disabled, err)
	}

	reboundCommand := command
	reboundCommand.IdempotencyKey = "push-subscription-owned-rebind"
	reboundCommand.CommandFingerprint = strings.Repeat("e", 64)
	reboundCommand.P256DHCiphertext = bytes.Repeat([]byte("r"), 96)
	reboundCommand.AuthCiphertext = bytes.Repeat([]byte("s"), 64)
	reboundCommand.DeviceLabel = "重新关联的手机"
	reboundCommand.CreatedAt = now.Add(2 * time.Second)
	rebound, err := repository.PersistPushSubscription(ctx, reboundCommand)
	if err != nil || rebound.ID != created.ID || rebound.Version != disabled.Version+1 || rebound.Status != "active" ||
		rebound.DeviceLabel != "重新关联的手机" {
		t.Fatalf("rebind = %#v / %v", rebound, err)
	}

	otherUserID, otherMonitorID, _ := insertEmailNotificationFixture(t, runtime, now.Add(3*time.Second))
	foreign := reboundCommand
	foreign.UserID = otherUserID
	foreign.MonitorIDs = []int64{otherMonitorID}
	foreign.IdempotencyKey = "push-subscription-foreign-rebind"
	foreign.CommandFingerprint = strings.Repeat("d", 64)
	if _, err := repository.PersistPushSubscription(ctx, foreign); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("foreign endpoint error = %v", err)
	}
}

func pushSubscriptionPersistenceFixture(userID, monitorID, sequence int64, now time.Time) application.PersistPushSubscriptionCommand {
	digit := byte('a' + sequence%6)
	return application.PersistPushSubscriptionCommand{
		UserID: userID, EndpointSHA256: strings.Repeat(string(digit), 64),
		EndpointCiphertext: bytes.Repeat([]byte{digit}, 96), P256DHCiphertext: bytes.Repeat([]byte{digit + 1}, 96),
		AuthCiphertext: bytes.Repeat([]byte{digit + 2}, 64), EncryptionKeyVersion: 1,
		DeviceLabel: "测试设备", Timezone: "Asia/Shanghai", TTLSeconds: 3600, MonitorIDs: []int64{monitorID},
		IdempotencyKey: "push-subscription-integration-" + string(digit), CommandFingerprint: strings.Repeat(string(digit+1), 64),
		CreatedAt: now,
	}
}

func insertPushUser(t *testing.T, runtime *database.Runtime, now time.Time) int64 {
	t.Helper()
	var userID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','其他推送用户','viewer') RETURNING id`, "push-other-"+strings.ReplaceAll(now.Format(time.RFC3339Nano), ":", "-")+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	return userID
}
