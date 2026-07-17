//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestDeliveryRepositoryIsIdempotentAndAppendsAttempts(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var userID, eventID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, display_name, role) VALUES ('delivery-' || md5(random()::text) || '@example.test', 'hash', 'delivery', 'viewer') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('delivery-event-' || md5(random()::text), 'Delivery event', '', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO reports (id, report_type, period_start, period_end, timezone, title, status, version_no) VALUES (8101, 'daily', $1, $2, 'UTC', 'Delivery report', 'published', 1)`, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_items (report_id, event_id, rank, section, title_snapshot, summary_snapshot, heat_score_snapshot) VALUES (8101, $1, 1, 'events', 'Delivery event', 'Snapshot', 90)`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_subscriptions (id, user_id, report_type, channel, recipient, timezone, schedule) VALUES (8201, $1, 'daily', 'email', 'delivery@example.test', 'UTC', '0 8 * * *')`, userID); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	subscription := domain.Subscription{ID: 8201, Version: 1, UserID: userID, ReportType: "daily", Channel: domain.ChannelEmail, Recipient: "delivery@example.test", Timezone: "UTC", Schedule: "0 8 * * *", Enabled: true}
	if err := repository.SaveSubscription(ctx, subscription); err != nil {
		t.Fatal(err)
	}
	rssSubscription := domain.Subscription{ID: 8202, Version: 1, UserID: userID, ReportType: "daily", Channel: domain.ChannelRSS, TokenHash: domain.TokenHash("secret"), Timezone: "UTC", Schedule: "0 8 * * *", Enabled: true}
	if err := repository.SaveSubscription(ctx, rssSubscription); err != nil {
		t.Fatal(err)
	}
	delivery := domain.Delivery{ID: 8301, ReportID: 8101, SubscriptionID: 8201, IdempotencyKey: "delivery-8101-8201", Status: domain.DeliveryQueued, NextAttemptAt: &now}
	created, err := repository.CreateDelivery(ctx, delivery)
	if err != nil || !created {
		t.Fatalf("CreateDelivery() = %v/%v, want created", created, err)
	}
	delivery.ID = 8302
	created, err = repository.CreateDelivery(ctx, delivery)
	if err != nil || created {
		t.Fatalf("duplicate CreateDelivery() = %v/%v, want no-op", created, err)
	}
	if err := repository.AppendAttempt(ctx, 8301, 1, "failed", 421, "temporary smtp failure"); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM delivery_attempts WHERE delivery_id = 8301`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	feed, err := repository.ReadFeed(ctx, rssSubscription.TokenHash)
	if err != nil || feed.Title != "Delivery report" || len(feed.Items) != 1 {
		t.Fatalf("ReadFeed() = %#v/%v", feed, err)
	}
}
