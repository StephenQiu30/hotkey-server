//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	notificationjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/infrastructure/jobs"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
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
	service, err := application.NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := notificationjobs.NewOutboxProjectionHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	inputHash := queue.StableJobHash(queue.KindProjectUserNotification, fmt.Sprint(outboxID), "1")
	job := queue.Job{
		Kind:        queue.KindProjectUserNotification,
		UniqueKey:   queue.StableJobKey(queue.KindProjectUserNotification, outboxID, 1, inputHash),
		Payload:     queue.Payload{EntityID: outboxID, EntityVersion: 1, InputHash: inputHash},
		ScheduledAt: now, MaxAttempts: 5, Priority: 6,
	}
	store := queue.NewStore(runtime)
	firstJobID, created, err := store.Enqueue(ctx, job)
	if err != nil || !created || firstJobID <= 0 {
		t.Fatalf("first projection job = %d/%t/%v", firstJobID, created, err)
	}
	replayedJobID, created, err := store.Enqueue(ctx, job)
	if err != nil || created || replayedJobID != firstJobID {
		t.Fatalf("replayed projection job = %d/%t/%v", replayedJobID, created, err)
	}
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindProjectUserNotification: handler.Handle})
	if ran, err := worker.RunOnce(ctx); err != nil || !ran {
		t.Fatalf("projection worker = %t/%v", ran, err)
	}
	if err := runtime.SQL.QueryRow(`SELECT id FROM user_notifications WHERE outbox_event_id=$1 AND user_id=$2`, outboxID, userID).Scan(&notificationID); err != nil {
		t.Fatal(err)
	}
	replayedProjection, err := repository.ProjectUserNotification(ctx, application.ProjectUserNotificationCommand{OutboxEventID: outboxID, OutboxVersion: 1})
	if err != nil || replayedProjection.Created || replayedProjection.UserNotificationID != notificationID {
		t.Fatalf("replayed outbox projection = %#v / %v", replayedProjection, err)
	}
	firstReceipt, err := repository.RecordNotificationReadReceipt(ctx, application.RecordNotificationReadReceiptCommand{
		UserID: userID, ReadThroughID: notificationID,
	})
	if err != nil || firstReceipt.ReceiptID <= 0 || firstReceipt.ReadThroughID != notificationID || !firstReceipt.Advanced {
		t.Fatalf("first read receipt = %#v / %v", firstReceipt, err)
	}
	replayedReceipt, err := repository.RecordNotificationReadReceipt(ctx, application.RecordNotificationReadReceiptCommand{
		UserID: userID, ReadThroughID: notificationID,
	})
	if err != nil || replayedReceipt.ReceiptID != firstReceipt.ReceiptID || replayedReceipt.Advanced {
		t.Fatalf("replayed read receipt = %#v / %v", replayedReceipt, err)
	}

	var secondOutboxID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,2,$2,$3,'事件再次更新','新增第二条通知','active',$4,$5) RETURNING id`,
		eventID, monitorID, now.Add(time.Minute), fmt.Sprintf("/dashboard/events?event=%d", eventID),
		fmt.Sprintf("notification-fixture-second:%d", now.UnixNano())).Scan(&secondOutboxID); err != nil {
		t.Fatal(err)
	}
	secondProjection, err := repository.ProjectUserNotification(ctx, application.ProjectUserNotificationCommand{OutboxEventID: secondOutboxID, OutboxVersion: 1})
	if err != nil || !secondProjection.Created || secondProjection.UserNotificationID <= notificationID {
		t.Fatalf("second outbox projection = %#v / %v", secondProjection, err)
	}
	secondReceipt, err := repository.RecordNotificationReadReceipt(ctx, application.RecordNotificationReadReceiptCommand{
		UserID: userID, ReadThroughID: secondProjection.UserNotificationID,
	})
	if err != nil || !secondReceipt.Advanced || secondReceipt.ReadThroughID != secondProjection.UserNotificationID {
		t.Fatalf("second read receipt = %#v / %v", secondReceipt, err)
	}
	regressed, err := repository.RecordNotificationReadReceipt(ctx, application.RecordNotificationReadReceiptCommand{
		UserID: userID, ReadThroughID: notificationID,
	})
	if !errors.Is(err, sharedrepository.ErrConflict) || regressed.ReadThroughID != secondProjection.UserNotificationID {
		t.Fatalf("regressed read receipt = %#v / %v", regressed, err)
	}
	unauthorized, err := repository.RecordNotificationReadReceipt(ctx, application.RecordNotificationReadReceiptCommand{
		UserID: otherUserID, ReadThroughID: secondProjection.UserNotificationID,
	})
	if !errors.Is(err, sharedrepository.ErrNotFound) || unauthorized.ReadThroughID != 0 {
		t.Fatalf("cross-user read receipt = %#v / %v", unauthorized, err)
	}

	page, err := repository.ListUserNotifications(ctx, application.ListUserNotificationsQuery{UserID: userID, Limit: 10})
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != notificationID || page.ReadThroughID != secondProjection.UserNotificationID ||
		page.Items[0].DeepLink != fmt.Sprintf("/dashboard/events?event=%d", eventID) {
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
	if _, err := runtime.SQL.Exec(`UPDATE notification_read_receipts SET read_through_id=$1 WHERE id=$2`, notificationID, secondReceipt.ReceiptID); err == nil {
		t.Fatal("append-only notification read receipt accepted mutation")
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='archived' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}
	page, err = repository.ListUserNotifications(ctx, application.ListUserNotificationsQuery{UserID: userID, Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("revoked monitor page = %#v / %v", page, err)
	}
}
