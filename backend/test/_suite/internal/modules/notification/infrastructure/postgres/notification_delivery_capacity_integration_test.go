//go:build integration

package postgres

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

const (
	notificationCapacityFacts   = 100
	notificationCapacityDevices = 4
	notificationCapacityWorkers = 8
)

// TestNotificationDeliveryCapacityBaseline is intentionally a bounded,
// reproducible capacity gate rather than a production sizing claim. It proves
// that the current PostgreSQL queue can drain one notification fan-out window
// concurrently without duplicate delivery facts or an unbounded storage jump.
func TestNotificationDeliveryCapacityBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
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
	insertNotificationCapacityFacts(t, runtime, userID, monitorID, now, notificationCapacityFacts-1)
	for sequence := int64(1); sequence <= notificationCapacityDevices; sequence++ {
		mustPersistPushSubscription(t, repository, pushSubscriptionPersistenceFixture(userID, monitorID, sequence, now))
	}

	var beforeBytes int64
	if err := runtime.SQL.QueryRow(`SELECT pg_total_relation_size('notification_delivery_attempts')`).Scan(&beforeBytes); err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	var tokenSequence atomic.Uint64
	var delivered atomic.Int64
	errorsChannel := make(chan error, notificationCapacityWorkers)
	var workers sync.WaitGroup
	for worker := 0; worker < notificationCapacityWorkers; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				sequence := tokenSequence.Add(1)
				token := fmt.Sprintf("%064x", sequence)
				claimed, claimErr := repository.ClaimNextWebPushDelivery(ctx, application.ClaimNextWebPushDeliveryCommand{
					ClaimToken: token, ClaimedAt: now, LeaseUntil: now.Add(time.Minute),
				})
				if claimErr != nil {
					errorsChannel <- fmt.Errorf("claim next delivery: %w", claimErr)
					return
				}
				if !claimed.Claimed {
					return
				}
				if _, completeErr := repository.CompleteWebPushDelivery(ctx, application.CompleteWebPushDeliveryCommand{
					UserNotificationID: claimed.Notification.ID, UserID: claimed.Notification.UserID,
					SubscriptionID: claimed.SubscriptionID, ClaimToken: token, Status: "succeeded",
					ProviderMessageID: application.WebPushDeliveryTarget(claimed.SubscriptionID),
					AttemptedAt:       now.Add(time.Second),
				}); completeErr != nil {
					errorsChannel <- fmt.Errorf("complete delivery: %w", completeErr)
					return
				}
				delivered.Add(1)
			}
		}()
	}
	workers.Wait()
	close(errorsChannel)
	for workerErr := range errorsChannel {
		if workerErr != nil {
			t.Fatal(workerErr)
		}
	}
	expectedDeliveries := int64(notificationCapacityFacts * notificationCapacityDevices)
	if delivered.Load() != expectedDeliveries {
		t.Fatalf("delivered %d, want %d", delivered.Load(), expectedDeliveries)
	}
	if elapsed := time.Since(startedAt); elapsed > 15*time.Second {
		t.Fatalf("bounded delivery window took %s, want <= 15s", elapsed)
	}

	var attempts, uniqueTargets, outstandingClaims int64
	if err := runtime.SQL.QueryRow(`
SELECT count(*),count(DISTINCT (user_notification_id,delivery_target_key)),
       (SELECT count(*) FROM notification_delivery_claims WHERE channel='web_push')
FROM notification_delivery_attempts WHERE channel='web_push'`).Scan(&attempts, &uniqueTargets, &outstandingClaims); err != nil {
		t.Fatal(err)
	}
	if attempts != expectedDeliveries || uniqueTargets != expectedDeliveries || outstandingClaims != 0 {
		t.Fatalf("delivery facts attempts=%d unique=%d claims=%d", attempts, uniqueTargets, outstandingClaims)
	}
	var afterBytes int64
	if err := runtime.SQL.QueryRow(`SELECT pg_total_relation_size('notification_delivery_attempts')`).Scan(&afterBytes); err != nil {
		t.Fatal(err)
	}
	if growthPerDelivery := (afterBytes - beforeBytes) / expectedDeliveries; growthPerDelivery <= 0 || growthPerDelivery > 16*1024 {
		t.Fatalf("delivery storage growth = %d bytes/fact, want 1..16384", growthPerDelivery)
	}
}

func insertNotificationCapacityFacts(
	t *testing.T,
	runtime *database.Runtime,
	userID, monitorID int64,
	now time.Time,
	count int,
) {
	t.Helper()
	if count <= 0 {
		return
	}
	var eventID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO micro_events(event_key,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version)
VALUES ($1,'subject:capacity','action:updated',$2,'micro-event-clustering-v1') RETURNING id`,
		fmt.Sprintf("%064x", now.UnixNano()+999), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	deepLink := fmt.Sprintf("/dashboard/events?event=%d", eventID)
	if _, err := runtime.SQL.Exec(`
WITH outbox AS (
    INSERT INTO notification_outbox_events(
        event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,
        title,summary,resource_status,deep_link,dedupe_key
    )
	    SELECT 'micro_event.updated','micro_event',$1::bigint,1,$2::bigint,
	           $3::timestamptz + generated.n * interval '1 millisecond',
	           '容量基线事件','不含正文的安全摘要','active',$4::varchar,
	           'notification-capacity:' || ($1::bigint)::text || ':' || generated.n::text
	    FROM generate_series(1,$5::integer) AS generated(n)
    RETURNING id,occurred_at
)
INSERT INTO user_notifications(
    outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
    occurred_at,title,summary,resource_status,deep_link
)
	SELECT outbox.id,$6::bigint,$2::bigint,'micro_event.updated','micro_event',$1::bigint,1,outbox.occurred_at,
	       '容量基线事件','不含正文的安全摘要','active',$4::varchar
	FROM outbox`, eventID, monitorID, now, deepLink, count, userID); err != nil {
		t.Fatal(err)
	}
}
