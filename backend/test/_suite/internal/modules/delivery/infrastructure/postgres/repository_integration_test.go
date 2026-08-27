//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
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
	var userID, eventID, eventUpdateID, monitorID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, display_name, role) VALUES ('delivery-' || md5(random()::text) || '@example.test', 'hash', 'delivery', 'viewer') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('delivery-event-' || md5(random()::text), 'Delivery event', '', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO event_updates (event_id, sequence_no, kind, summary, observed_at, reason_codes, before_state, after_state, evidence_set_hash, idempotency_key) VALUES ($1, 1, 'event_created', 'Delivery snapshot', $2, ARRAY['delivery_fixture'], '{}', '{"heat_score":90}', repeat('a',64), repeat('b',64)) RETURNING id`, eventID, now).Scan(&eventUpdateID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO reports (id, report_type, period_start, period_end, timezone, title, status, version_no, created_by, updated_by) VALUES (8101, 'daily', $1, $2, 'UTC', 'Delivery report', 'draft', 1, $3, $3)`, now, now.Add(time.Hour), userID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_items (report_id, event_id, event_update_id, rank, section, title_snapshot, summary_snapshot, heat_score_snapshot, evidence_set_hash, reason_codes) VALUES (8101, $1, $2, 1, 'events', 'Delivery event', 'Snapshot', 90, repeat('a',64), ARRAY['delivery_fixture'])`, eventID, eventUpdateID); err != nil {
		t.Fatal(err)
	}
	publishReportFixture(t, ctx, runtime, 8101, userID)
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO monitors (name, status) VALUES ('delivery-scope-' || md5(random()::text), 'active') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO reports (id, monitor_id, report_type, period_start, period_end, timezone, title, status, version_no, created_by, updated_by) VALUES (8102, $1, 'daily', $2, $3, 'UTC', 'Scoped report', 'draft', 1, $4, $4)`, monitorID, now.Add(time.Hour), now.Add(2*time.Hour), userID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_items (report_id, event_id, event_update_id, rank, section, title_snapshot, summary_snapshot, heat_score_snapshot, evidence_set_hash, reason_codes) VALUES (8102, $1, $2, 1, 'events', 'Scoped event', 'Scoped snapshot', 95, repeat('a',64), ARRAY['delivery_fixture'])`, eventID, eventUpdateID); err != nil {
		t.Fatal(err)
	}
	publishReportFixture(t, ctx, runtime, 8102, userID)
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
	scopedRSSSubscription := domain.Subscription{ID: 8203, Version: 1, UserID: userID, MonitorID: &monitorID, ReportType: "daily", Channel: domain.ChannelRSS, TokenHash: domain.TokenHash("scoped"), Timezone: "UTC", Schedule: "0 8 * * *", Enabled: true}
	if err := repository.SaveSubscription(ctx, scopedRSSSubscription); err != nil {
		t.Fatal(err)
	}
	createdSubscription, err := repository.CreateSubscription(ctx, domain.Subscription{UserID: userID, ReportType: "weekly", Channel: domain.ChannelRSS, TokenHash: domain.TokenHash("rotated"), Timezone: "Asia/Shanghai", Schedule: "0 9 * * 1", Enabled: true})
	if err != nil || createdSubscription.ID <= 0 || createdSubscription.Version != 1 {
		t.Fatalf("CreateSubscription() = %#v/%v", createdSubscription, err)
	}
	rotatedSubscription, err := repository.RotateRSSToken(ctx, createdSubscription.ID, userID, createdSubscription.Version, domain.TokenHash("rotated-again"))
	if err != nil || rotatedSubscription.Version != 2 || rotatedSubscription.TokenHash != domain.TokenHash("rotated-again") {
		t.Fatalf("RotateRSSToken() = %#v/%v", rotatedSubscription, err)
	}
	page, err := repository.ListSubscriptions(ctx, userID, domain.SubscriptionListQuery{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("ListSubscriptions(first page) = %#v/%v", page, err)
	}
	if _, err := repository.ListSubscriptions(ctx, userID+999, domain.SubscriptionListQuery{Cursor: page.NextCursor, Limit: 2}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-user subscription cursor error = %v", err)
	}
	nextPage, err := repository.ListSubscriptions(ctx, userID, domain.SubscriptionListQuery{Cursor: page.NextCursor, Limit: 20})
	if err != nil || len(nextPage.Items) != 2 || nextPage.NextCursor != "" {
		t.Fatalf("ListSubscriptions(next page) = %#v/%v", nextPage, err)
	}
	if page.Items[0].ID <= page.Items[1].ID || page.Items[1].ID <= nextPage.Items[0].ID {
		t.Fatalf("subscription cursor order = first %#v next %#v", page.Items, nextPage.Items)
	}
	delivery := domain.Delivery{ReportID: 8101, SubscriptionID: 8201, IdempotencyKey: "delivery-8101-8201", Status: domain.DeliveryQueued, NextAttemptAt: &now}
	created, err := repository.CreateDelivery(ctx, delivery)
	if err != nil || !created {
		t.Fatalf("CreateDelivery() = %v/%v, want created", created, err)
	}
	allocated, err := repository.GetDeliveryForScope(ctx, delivery.ReportID, delivery.SubscriptionID)
	if err != nil || allocated.ID <= 0 {
		t.Fatalf("allocated delivery = %#v/%v", allocated, err)
	}
	delivery.ID = 8302
	created, err = repository.CreateDelivery(ctx, delivery)
	if err != nil || created {
		t.Fatalf("duplicate CreateDelivery() = %v/%v, want no-op", created, err)
	}
	if err := repository.AppendAttempt(ctx, allocated.ID, 1, "failed", 421, "temporary smtp failure"); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM delivery_attempts WHERE delivery_id = $1`, allocated.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	feed, err := repository.ReadFeed(ctx, rssSubscription.TokenHash)
	if err != nil || feed.Title != "Delivery report" || len(feed.Items) != 1 {
		t.Fatalf("ReadFeed() = %#v/%v", feed, err)
	}
	scopedFeed, err := repository.ReadFeed(ctx, scopedRSSSubscription.TokenHash)
	if err != nil || scopedFeed.Title != "Scoped report" || len(scopedFeed.Items) != 1 {
		t.Fatalf("Read scoped feed() = %#v/%v", scopedFeed, err)
	}
}

func publishReportFixture(t *testing.T, ctx context.Context, runtime *database.Runtime, reportID, actorID int64) {
	t.Helper()
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE reports SET version=2,status='pending_approval',submitted_at=now(),submitted_by=$2,updated_by=$2 WHERE id=$1`, reportID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_revision_transitions (report_id,from_status,to_status,expected_resource_version,result_resource_version,actor_user_id) VALUES ($1,'draft','pending_approval',1,2,$2)`, reportID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE reports SET version=3,status='published',published_at=now(),reviewed_at=now(),reviewed_by=$2,updated_by=$2 WHERE id=$1`, reportID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_revision_transitions (report_id,from_status,to_status,expected_resource_version,result_resource_version,actor_user_id) VALUES ($1,'pending_approval','published',2,3,$2)`, reportID, actorID); err != nil {
		t.Fatal(err)
	}
}
