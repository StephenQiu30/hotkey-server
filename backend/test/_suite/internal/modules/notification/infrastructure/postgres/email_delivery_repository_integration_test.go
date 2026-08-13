//go:build integration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
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
	userID, monitorID, notificationID := insertEmailNotificationFixture(t, runtime, now)

	first, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("a", 64), ClaimedAt: now, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil || !first.Claimed || first.Notification.ID != notificationID || first.Notification.UserID != userID ||
		first.AttemptCount != 0 || first.RecipientEmail == "" || !first.AlertEmailEnabled {
		t.Fatalf("first claim = %#v / %v", first, err)
	}
	concurrent, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("b", 64), ClaimedAt: now.Add(time.Second), LeaseUntil: now.Add(time.Minute + time.Second),
	})
	if err != nil || concurrent.Claimed {
		t.Fatalf("concurrent claim = %#v / %v", concurrent, err)
	}
	failed, err := repository.CompleteEmailDelivery(ctx, application.CompleteEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: strings.Repeat("a", 64),
		Status: "failed", ErrorCode: "smtp_temporary", AttemptedAt: now.Add(2 * time.Second),
	})
	if err != nil || failed.AttemptNo != 1 {
		t.Fatalf("failed completion = %#v / %v", failed, err)
	}
	immediate, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("c", 64), ClaimedAt: now.Add(30 * time.Second), LeaseUntil: now.Add(90 * time.Second),
	})
	if err != nil || immediate.Claimed {
		t.Fatalf("backoff claim = %#v / %v", immediate, err)
	}
	retryAt := now.Add(time.Minute + 3*time.Second)
	retry, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("d", 64), ClaimedAt: retryAt, LeaseUntil: retryAt.Add(time.Minute),
	})
	if err != nil || !retry.Claimed || retry.AttemptCount != 1 {
		t.Fatalf("retry claim = %#v / %v", retry, err)
	}
	succeeded, err := repository.CompleteEmailDelivery(ctx, application.CompleteEmailDeliveryCommand{
		UserNotificationID: notificationID, UserID: userID, ClaimToken: strings.Repeat("d", 64),
		Status: "succeeded", ProviderMessageID: "smtp-fixture", AttemptedAt: retryAt.Add(time.Second),
	})
	if err != nil || succeeded.AttemptNo != 2 {
		t.Fatalf("successful completion = %#v / %v", succeeded, err)
	}
	terminal, err := repository.ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("e", 64), ClaimedAt: retryAt.Add(10 * time.Minute), LeaseUntil: retryAt.Add(11 * time.Minute),
	})
	if err != nil || terminal.Claimed {
		t.Fatalf("terminal claim = %#v / %v", terminal, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE notification_delivery_attempts SET status='failed' WHERE user_notification_id=$1`, notificationID); err == nil {
		t.Fatal("append-only delivery attempt accepted mutation")
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='archived' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}
}

func insertEmailNotificationFixture(t *testing.T, runtime *database.Runtime, now time.Time) (int64, int64, int64) {
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
VALUES ($1,1,'draft',ARRAY['zh'],true,$2,$2) RETURNING id`, monitorID, userID).Scan(&configID); err != nil {
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
