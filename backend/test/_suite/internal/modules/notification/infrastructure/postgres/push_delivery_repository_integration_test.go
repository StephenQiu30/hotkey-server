//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestWebPushDeliveryRepositorySeparatesDevicesAndExpiresOnlyGoneTarget(t *testing.T) {
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
	userID, monitorID, notificationID := insertEmailNotificationFixture(t, runtime, now)
	first := mustPersistPushSubscription(t, repository, pushSubscriptionPersistenceFixture(userID, monitorID, 1, now))
	second := mustPersistPushSubscription(t, repository, pushSubscriptionPersistenceFixture(userID, monitorID, 2, now))

	firstClaim := mustClaimWebPush(t, repository, now, "a")
	if firstClaim.SubscriptionID != first.ID || firstClaim.Notification.ID != notificationID || firstClaim.AttemptCount != 0 {
		t.Fatalf("first claim = %#v", firstClaim)
	}
	if err := repository.ValidateWebPushDeliveryClaim(ctx, application.ValidateWebPushDeliveryClaimQuery{
		UserNotificationID: notificationID, SubscriptionID: first.ID, ClaimToken: strings.Repeat("a", 64),
		ValidatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	goneCode := 410
	completed, err := repository.CompleteWebPushDelivery(ctx, application.CompleteWebPushDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, SubscriptionID: first.ID, ClaimToken: strings.Repeat("a", 64),
		Status: "permanent_failure", ProviderMessageID: application.WebPushDeliveryTarget(first.ID), ResponseCode: &goneCode,
		ErrorCode: "push_subscription_gone", ExpirationReason: "push_service_gone", AttemptedAt: now.Add(2 * time.Second),
	})
	if err != nil || completed.AttemptNo != 1 {
		t.Fatalf("gone completion = %#v / %v", completed, err)
	}

	secondClaim := mustClaimWebPush(t, repository, now.Add(3*time.Second), "b")
	if secondClaim.SubscriptionID != second.ID || secondClaim.AttemptCount != 0 {
		t.Fatalf("second claim = %#v", secondClaim)
	}
	completed, err = repository.CompleteWebPushDelivery(ctx, application.CompleteWebPushDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, SubscriptionID: second.ID, ClaimToken: strings.Repeat("b", 64),
		Status: "succeeded", ProviderMessageID: "push-provider-second", ResponseCode: integerPointer(201),
		AttemptedAt: now.Add(4 * time.Second),
	})
	if err != nil || completed.AttemptNo != 1 {
		t.Fatalf("success completion = %#v / %v", completed, err)
	}
	var firstStatus, secondStatus string
	if err := runtime.SQL.QueryRow(`SELECT status FROM web_push_subscriptions WHERE id=$1`, first.ID).Scan(&firstStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT status FROM web_push_subscriptions WHERE id=$1`, second.ID).Scan(&secondStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != "expired" || secondStatus != "active" {
		t.Fatalf("device statuses = %q/%q", firstStatus, secondStatus)
	}
	var targetCount int
	if err := runtime.SQL.QueryRow(`SELECT count(DISTINCT delivery_target_key) FROM notification_delivery_attempts
WHERE user_notification_id=$1 AND channel='web_push'`, notificationID).Scan(&targetCount); err != nil || targetCount != 2 {
		t.Fatalf("delivery target count = %d / %v", targetCount, err)
	}

	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='archived' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateWebPushDeliveryClaim(ctx, application.ValidateWebPushDeliveryClaimQuery{
		UserNotificationID: notificationID, SubscriptionID: second.ID, ClaimToken: strings.Repeat("b", 64),
		ValidatedAt: now.Add(5 * time.Second),
	}); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("revoked validation error = %v", err)
	}
}

func TestWebPushDeliveryRepositoryHonorsRetryBackoffAndQuietHours(t *testing.T) {
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
	userID, monitorID, notificationID := insertEmailNotificationFixture(t, runtime, now)
	subscription := mustPersistPushSubscription(t, repository, pushSubscriptionPersistenceFixture(userID, monitorID, 3, now))
	claim := mustClaimWebPush(t, repository, now, "c")
	if claim.SubscriptionID != subscription.ID {
		t.Fatalf("claim = %#v", claim)
	}
	if _, err := repository.CompleteWebPushDelivery(ctx, application.CompleteWebPushDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, SubscriptionID: subscription.ID, ClaimToken: strings.Repeat("c", 64),
		Status: "failed", ErrorCode: "push_temporary", AttemptedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	tooSoon, err := repository.ClaimNextWebPushDelivery(ctx, application.ClaimNextWebPushDeliveryCommand{
		ClaimToken: strings.Repeat("d", 64), ClaimedAt: now.Add(30 * time.Second), LeaseUntil: now.Add(90 * time.Second),
	})
	if err != nil || tooSoon.Claimed {
		t.Fatalf("early retry = %#v / %v", tooSoon, err)
	}
	retry := mustClaimWebPush(t, repository, now.Add(62*time.Second), "e")
	if retry.AttemptCount != 1 || retry.SubscriptionID != subscription.ID {
		t.Fatalf("retry = %#v", retry)
	}
	if _, err := repository.CompleteWebPushDelivery(ctx, application.CompleteWebPushDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, SubscriptionID: subscription.ID, ClaimToken: strings.Repeat("e", 64),
		Status: "succeeded", ProviderMessageID: "push-retry", AttemptedAt: now.Add(63 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	// A fresh notification is blocked by an all-day window containing the fixture time.
	secondNotificationID := insertPushNotification(t, runtime, userID, monitorID, now.Add(64*time.Second))
	local := now.In(time.UTC)
	quietStart := local.Add(-time.Hour).Format("15:04")
	quietEnd := local.Add(time.Hour).Format("15:04")
	if _, err := repository.UpdatePushSubscription(ctx, application.UpdatePushSubscriptionCommand{
		UserID: userID, SubscriptionID: subscription.ID, ExpectedVersion: subscription.Version,
		DeviceLabel: "安静设备", Timezone: "UTC", QuietStart: &quietStart, QuietEnd: &quietEnd,
		TTLSeconds: 3600, MonitorIDs: []int64{monitorID}, UpdatedAt: now.Add(64 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	quietClaim, err := repository.ClaimNextWebPushDelivery(ctx, application.ClaimNextWebPushDeliveryCommand{
		ClaimToken: strings.Repeat("f", 64), ClaimedAt: now.Add(65 * time.Second), LeaseUntil: now.Add(125 * time.Second),
	})
	if err != nil || quietClaim.Claimed {
		t.Fatalf("quiet claim for notification %d = %#v / %v", secondNotificationID, quietClaim, err)
	}
}

func mustPersistPushSubscription(t *testing.T, repository *Repository, command application.PersistPushSubscriptionCommand) application.PushSubscriptionDTO {
	t.Helper()
	result, err := repository.PersistPushSubscription(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustClaimWebPush(t *testing.T, repository *Repository, now time.Time, tokenCharacter string) application.ClaimedWebPushDeliveryDTO {
	t.Helper()
	result, err := repository.ClaimNextWebPushDelivery(context.Background(), application.ClaimNextWebPushDeliveryCommand{
		ClaimToken: strings.Repeat(tokenCharacter, 64), ClaimedAt: now, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Claimed {
		t.Fatal("expected Web Push delivery claim")
	}
	return result
}

func integerPointer(value int) *int { return &value }

func insertPushNotification(t *testing.T, runtime *database.Runtime, userID, monitorID int64, now time.Time) int64 {
	t.Helper()
	var eventID, outboxID, notificationID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO micro_events(
event_key,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version)
VALUES ($1,'subject:push','action:updated',$2,'micro-event-clustering-v1') RETURNING id`,
		fmt.Sprintf("%064x", now.UnixNano()), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	deepLink := fmt.Sprintf("/dashboard/events?event=%d", eventID)
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,1,$2,$3,'安静时段事件','安全摘要','active',$4,$5) RETURNING id`,
		eventID, monitorID, now, deepLink, fmt.Sprintf("notification-push:%d", now.UnixNano())).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO user_notifications(
outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,1,$5,'安静时段事件','安全摘要','active',$6)
RETURNING id`, outboxID, userID, monitorID, eventID, now, deepLink).Scan(&notificationID); err != nil {
		t.Fatal(err)
	}
	return notificationID
}
