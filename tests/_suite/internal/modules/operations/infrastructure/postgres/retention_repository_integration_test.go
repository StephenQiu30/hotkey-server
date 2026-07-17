//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRetentionRepositoryArchivesBusinessRowsAndDeletesAttempts(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour)
	var sourceID, contentID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'retention-' || md5(random()::text), 'https://retention.example') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO contents (source_connection_id, external_id, content_type, title, canonical_url, published_at, fetched_at, dedupe_key, created_at) VALUES ($1, 'retention-content', 'article', 'old', 'https://retention.example/content', $2, $2, repeat('r', 64), $2) RETURNING id`, sourceID, old).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	repository := NewRetentionRepository(runtime)
	affected, err := repository.ApplyRetention(ctx, operationsdomain.RetentionPolicy{ID: 1, Version: 1, DataClass: "contents", RetentionDays: 1, Action: "archive", Enabled: true}, time.Now().UTC().Add(-24*time.Hour))
	if err != nil || affected != 1 {
		t.Fatalf("archive contents = %d/%v, want 1", affected, err)
	}
	var deletedAt *time.Time
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT deleted_at FROM contents WHERE id = $1`, contentID).Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if deletedAt == nil {
		t.Fatal("content was not archived")
	}
	var userID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, display_name, role) VALUES ('retention-' || md5(random()::text) || '@example.test', 'hash', 'retention', 'viewer') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO reports (id, report_type, period_start, period_end, timezone, title, status, version_no) VALUES (9201, 'daily', $1, $2, 'UTC', 'retention', 'published', 1)`, old, old.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO report_subscriptions (id, user_id, report_type, channel, recipient, timezone, schedule) VALUES (9301, $1, 'daily', 'email', 'retention@example.test', 'UTC', '0 8 * * *')`, userID); err != nil {
		t.Fatal(err)
	}
	var deliveryID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO report_deliveries (report_id, subscription_id, idempotency_key, status) VALUES (9201, 9301, 'retention-delivery', 'failed') RETURNING id`).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO delivery_attempts (delivery_id, attempt_no, started_at, status, created_at) VALUES ($1, 1, $2, 'failed', $2)`, deliveryID, old); err != nil {
		t.Fatal(err)
	}
	deleted, err := repository.ApplyRetention(ctx, operationsdomain.RetentionPolicy{ID: 2, Version: 1, DataClass: "delivery_attempts", RetentionDays: 1, Action: "delete", Enabled: true}, time.Now().UTC().Add(-24*time.Hour))
	if err != nil || deleted != 1 {
		t.Fatalf("delete attempts = %d/%v, want 1", deleted, err)
	}
}
