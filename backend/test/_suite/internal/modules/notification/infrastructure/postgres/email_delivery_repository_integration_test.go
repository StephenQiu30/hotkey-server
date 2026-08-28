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

func TestEmailDeliveryRepositoryClaimsWithCurrentPermissionBackoffAndTerminalAttempt(t *testing.T) {
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
	userID, monitorID, notificationID := insertEmailNotificationFixture(t, runtime, now, true)

	first, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("a", 64), LeaseDuration: time.Minute,
	})
	if err != nil || !first.Claimed || first.Notification.ID != notificationID || first.Notification.UserID != userID ||
		first.AttemptCount != 0 || first.RecipientEmail == "" || !first.AlertEmailEnabled || first.FencingGeneration != 1 ||
		len(first.DispatchKey) != 64 {
		t.Fatalf("first claim = %#v / %v", first, err)
	}
	var claimedAt, leaseUntil, databaseNow time.Time
	if err := runtime.SQL.QueryRow(`SELECT claimed_at,lease_until,clock_timestamp()
FROM notification_delivery_claims WHERE user_notification_id=$1 AND channel='email'`, notificationID).Scan(
		&claimedAt, &leaseUntil, &databaseNow,
	); err != nil || claimedAt.After(databaseNow) || leaseUntil.Sub(claimedAt) < 59*time.Second ||
		leaseUntil.Sub(claimedAt) > 61*time.Second {
		t.Fatalf("database lease = %s/%s/%s / %v", claimedAt, leaseUntil, databaseNow, err)
	}
	concurrent, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("b", 64), LeaseDuration: time.Minute,
	})
	if err != nil || concurrent.Claimed {
		t.Fatalf("concurrent claim = %#v / %v", concurrent, err)
	}
	if err := repository.StartEmailDelivery(ctx, application.StartEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: first.ClaimToken,
		FencingGeneration: first.FencingGeneration, DispatchKey: first.DispatchKey,
	}); err != nil {
		t.Fatalf("start email delivery: %v", err)
	}
	failed, err := repository.CompleteEmailDelivery(ctx, application.CompleteEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: strings.Repeat("a", 64),
		FencingGeneration: first.FencingGeneration, DispatchKey: first.DispatchKey,
		Status: "failed", ErrorCode: "smtp_temporary",
	})
	if err != nil || failed.AttemptNo != 1 {
		t.Fatalf("failed completion = %#v / %v", failed, err)
	}
	immediate, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("c", 64), LeaseDuration: time.Minute,
	})
	if err != nil || immediate.Claimed {
		t.Fatalf("backoff claim = %#v / %v", immediate, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE notification_delivery_attempts SET status='failed' WHERE user_notification_id=$1`, notificationID); err == nil {
		t.Fatal("append-only delivery attempt accepted mutation")
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='archived' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}
}

func TestEmailDeliveryRepositoryFencesExpiredWorkersAndStopsUnknownSMTPReplay(t *testing.T) {
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
	userID, monitorID, notificationID := insertEmailNotificationFixture(t, runtime, now, true)

	workerA, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("1", 64), LeaseDuration: time.Minute,
	})
	if err != nil || !workerA.Claimed || workerA.FencingGeneration != 1 {
		t.Fatalf("worker A claim = %#v / %v", workerA, err)
	}
	if err := repository.StartEmailDelivery(ctx, application.StartEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: workerA.ClaimToken,
		FencingGeneration: workerA.FencingGeneration, DispatchKey: workerA.DispatchKey,
	}); err != nil {
		t.Fatalf("worker A start: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE notification_delivery_claims
SET claimed_at=clock_timestamp()-interval '2 minutes',lease_until=clock_timestamp()-interval '1 second'
WHERE user_notification_id=$1 AND channel='email'`, notificationID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='archived' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}

	recovered, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("2", 64), LeaseDuration: time.Minute,
	})
	if err != nil || !recovered.Claimed || !recovered.RecoveredUnknown || recovered.AttemptCount != 1 {
		t.Fatalf("unsupported provider recovery = %#v / %v", recovered, err)
	}
	_, err = repository.CompleteEmailDelivery(ctx, application.CompleteEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: workerA.ClaimToken,
		FencingGeneration: workerA.FencingGeneration, DispatchKey: workerA.DispatchKey,
		Status: "succeeded", ProviderMessageID: "late-worker-a",
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("late worker A completion error = %v", err)
	}
	var status, dispatchKey, errorCode string
	var generation int64
	if err := runtime.SQL.QueryRow(`SELECT status,dispatch_key,fencing_generation,error_code
FROM notification_delivery_attempts WHERE user_notification_id=$1`, notificationID).Scan(
		&status, &dispatchKey, &generation, &errorCode,
	); err != nil || status != "unknown" || dispatchKey != workerA.DispatchKey || generation != 1 ||
		errorCode != "provider_outcome_unconfirmed" {
		t.Fatalf("unknown attempt = %q/%q/%d/%q / %v", status, dispatchKey, generation, errorCode, err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO notification_delivery_attempts(
user_notification_id,channel,delivery_target_key,attempt_no,status,error_code,attempted_at)
VALUES ($1,'email',$2,2,'unknown','forged_unknown',clock_timestamp())`,
		notificationID, application.PrimaryEmailDeliveryTarget); err == nil {
		t.Fatal("unknown delivery attempt without a dispatch fence was accepted")
	}
	terminal, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("3", 64), LeaseDuration: time.Minute,
	})
	if err != nil || terminal.Claimed {
		t.Fatalf("unknown terminal replay = %#v / %v", terminal, err)
	}
}

func TestEmailDeliveryRepositoryReusesDispatchKeyWithHigherFenceForIdempotentProvider(t *testing.T) {
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
	userID, _, notificationID := insertEmailNotificationFixture(t, runtime, now, true)
	capabilities := application.NotificationEmailProviderCapabilities{SupportsIdempotency: true}
	if _, err := runtime.SQL.Exec(`INSERT INTO notification_delivery_attempts(
user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,error_code,attempted_at)
VALUES ($1,'email',$2,1,'failed',$3,7,'provider_rejected',clock_timestamp()-interval '10 minutes')`,
		notificationID, application.PrimaryEmailDeliveryTarget, strings.Repeat("9", 64)); err != nil {
		t.Fatal(err)
	}

	workerA, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("4", 64), LeaseDuration: time.Minute, ProviderCapabilities: capabilities,
	})
	if err != nil || !workerA.Claimed || workerA.FencingGeneration != 8 {
		t.Fatalf("worker A claim = %#v / %v", workerA, err)
	}
	if err := repository.StartEmailDelivery(ctx, application.StartEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: workerA.ClaimToken,
		FencingGeneration: workerA.FencingGeneration, DispatchKey: workerA.DispatchKey,
	}); err != nil {
		t.Fatalf("worker A start: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE notification_delivery_claims
SET claimed_at=clock_timestamp()-interval '2 minutes',lease_until=clock_timestamp()-interval '1 second'
WHERE user_notification_id=$1 AND channel='email'`, notificationID); err != nil {
		t.Fatal(err)
	}

	workerB, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("5", 64), LeaseDuration: time.Minute, ProviderCapabilities: capabilities,
	})
	if err != nil || !workerB.Claimed || !workerB.ReconcileRequired || workerB.FencingGeneration != 9 ||
		workerB.DispatchKey != workerA.DispatchKey {
		t.Fatalf("worker B claim = %#v / %v", workerB, err)
	}
	_, err = repository.CompleteEmailDelivery(ctx, application.CompleteEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: workerA.ClaimToken,
		FencingGeneration: workerA.FencingGeneration, DispatchKey: workerA.DispatchKey,
		Status: "succeeded", ProviderMessageID: "late-worker-a",
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("late worker A completion error = %v", err)
	}
	if err := repository.StartEmailDelivery(ctx, application.StartEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: workerB.ClaimToken,
		FencingGeneration: workerB.FencingGeneration, DispatchKey: workerB.DispatchKey,
	}); err != nil {
		t.Fatalf("worker B start: %v", err)
	}
	completed, err := repository.CompleteEmailDelivery(ctx, application.CompleteEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: workerB.ClaimToken,
		FencingGeneration: workerB.FencingGeneration, DispatchKey: workerB.DispatchKey,
		ProviderCapabilities: capabilities,
		Status:               "succeeded", ProviderMessageID: "idempotent-provider-result",
	})
	if err != nil || completed.AttemptNo != 2 {
		t.Fatalf("worker B completion = %#v / %v", completed, err)
	}
	var generation int64
	var dispatchKey string
	var supportsIdempotency bool
	if err := runtime.SQL.QueryRow(`SELECT fencing_generation,dispatch_key,provider_supports_idempotency
FROM notification_delivery_attempts WHERE user_notification_id=$1 AND status='succeeded'`, notificationID).Scan(
		&generation, &dispatchKey, &supportsIdempotency,
	); err != nil || generation != 9 || dispatchKey != workerA.DispatchKey || !supportsIdempotency {
		t.Fatalf("fenced attempt = %d/%q/%t / %v", generation, dispatchKey, supportsIdempotency, err)
	}
}

func TestEmailDeliveryRepositoryRechecksCurrentChannelPreferenceBeforeClaim(t *testing.T) {
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
	_, _, notificationID := insertEmailNotificationFixture(t, runtime, now, false)

	claimed, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("f", 64), LeaseDuration: time.Minute,
	})
	if err != nil || claimed.Claimed {
		t.Fatalf("disabled current email preference claim = %#v / %v", claimed, err)
	}
	var attempts int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM notification_delivery_attempts WHERE user_notification_id=$1`, notificationID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("disabled preference attempts = %d/%v", attempts, err)
	}
}

func insertEmailNotificationFixture(t *testing.T, runtime *database.Runtime, now time.Time, emailEnabled bool) (int64, int64, int64) {
	t.Helper()
	var userID, monitorID, configID, eventID, outboxID, notificationID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','邮件通知用户','viewer') RETURNING id`, fmt.Sprintf("notification-email-%d@example.test", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors(name,status,created_by,updated_by)
VALUES ($1,'draft',$2,$2) RETURNING id`, fmt.Sprintf("email-monitor-%d", now.UnixNano()), userID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions(
monitor_id,revision,state,languages,alert_email_enabled,created_by,updated_by)
VALUES ($1,1,'draft',ARRAY['zh'],$3,$2,$2) RETURNING id`, monitorID, userID, emailEnabled).Scan(&configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state='published',config_hash=repeat('a',64),published_at=$1 WHERE id=$2`, now, configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='active',published_config_version_id=$1 WHERE id=$2`, configID, monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO micro_events(
event_key,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version)
VALUES ($1,'subject:email','action:updated',$2,'micro-event-clustering-v1') RETURNING id`,
		fmt.Sprintf("%064x", now.UnixNano()), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	deepLink := fmt.Sprintf("/dashboard/events?event=%d", eventID)
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,1,$2,$3,'邮件事件','安全摘要','urgent',$4,$5) RETURNING id`,
		eventID, monitorID, now, deepLink, fmt.Sprintf("notification-email:%d", now.UnixNano())).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO user_notifications(
outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,1,$5,'邮件事件','安全摘要','urgent',$6)
RETURNING id`, outboxID, userID, monitorID, eventID, now, deepLink).Scan(&notificationID); err != nil {
		t.Fatal(err)
	}
	return userID, monitorID, notificationID
}
