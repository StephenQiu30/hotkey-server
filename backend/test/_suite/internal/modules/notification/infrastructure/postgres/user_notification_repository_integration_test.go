//go:build integration

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestUserNotificationRepositoryReplaysByUserRechecksMonitorAccessAndSeparatesDeliveryAttempts(t *testing.T) {
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

	var userID, otherUserID, monitorID, eventID, outboxID, notificationID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','通知用户','viewer') RETURNING id`, fmt.Sprintf("notification-%d@example.test", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','其他用户','viewer') RETURNING id`, fmt.Sprintf("notification-other-%d@example.test", now.UnixNano())).Scan(&otherUserID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors(name,status,created_by) VALUES ($1,'active',$2) RETURNING id`,
		fmt.Sprintf("notification-monitor-%d", now.UnixNano()), userID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO micro_events(
event_key,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version)
VALUES ($1,'subject:fixture','action:updated',$2,'micro-event-clustering-v1') RETURNING id`,
		fmt.Sprintf("%064x", now.UnixNano()), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,1,$2,$3,'事件更新','新增独立正文谱系','active',$4,$5) RETURNING id`,
		eventID, monitorID, now, fmt.Sprintf("/dashboard/events?event=%d", eventID), fmt.Sprintf("notification-fixture:%d", now.UnixNano())).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO user_notifications(
outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,1,$5,'事件更新','新增独立正文谱系','active',$6)
RETURNING id`, outboxID, userID, monitorID, eventID, now, fmt.Sprintf("/dashboard/events?event=%d", eventID)).Scan(&notificationID); err != nil {
		t.Fatal(err)
	}

	page, err := repository.ListUserNotifications(ctx, application.ListUserNotificationsQuery{UserID: userID, Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != notificationID || page.Items[0].DeepLink != fmt.Sprintf("/dashboard/events?event=%d", eventID) {
		t.Fatalf("owner page = %#v / %v", page, err)
	}
	otherPage, err := repository.ListUserNotifications(ctx, application.ListUserNotificationsQuery{UserID: otherUserID, Limit: 10})
	if err != nil || len(otherPage.Items) != 0 {
		t.Fatalf("other user page = %#v / %v", otherPage, err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		result, err := repository.RecordDeliveryAttempt(ctx, application.RecordNotificationDeliveryAttemptCommand{
			UserNotificationID: notificationID, UserID: userID, Channel: "websocket", DeliveryTargetKey: "browser_ws",
			Status: "succeeded", AttemptedAt: now.Add(time.Duration(attempt) * time.Second),
		})
		if err != nil || result.AttemptNo != attempt {
			t.Fatalf("delivery attempt %d = %#v / %v", attempt, result, err)
		}
	}
	if _, err := runtime.SQL.Exec(`UPDATE user_notifications SET title='mutated' WHERE id=$1`, notificationID); err == nil {
		t.Fatal("append-only user notification accepted mutation")
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='archived' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}
	page, err = repository.ListUserNotifications(ctx, application.ListUserNotificationsQuery{UserID: userID, Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("revoked monitor page = %#v / %v", page, err)
	}
}
