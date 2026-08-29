//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetentionPolicySchemaRejectsAnEighthDataClass(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM retention_policies`).Scan(&count); err != nil || count != 7 {
		t.Fatalf("retention policy count = %d, %v; want fixed seven-item catalog", count, err)
	}
	_, err = runtime.SQL.ExecContext(ctx, `INSERT INTO retention_policies (data_class,retention_days,action) VALUES ('arbitrary_extra_class',30,'delete')`)
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("insert eighth retention policy error = %#v, want PostgreSQL CHECK violation", err)
	}
}

func TestRetentionRepositoryRejectsProtectedDeliveryAttemptsBeforeCreatingRun(t *testing.T) {
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
	repository := NewRetentionRepository(runtime)
	var userID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, display_name, role) VALUES ('retention-' || md5(random()::text) || '@example.test', 'hash', 'retention', 'viewer') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO reports (id, report_type, period_start, period_end, timezone, title, status, version_no, published_at, reviewed_at, reviewed_by, created_by, updated_by) VALUES (9201, 'daily', $1, $2, 'UTC', 'retention', 'published', 1, $1, $1, $3, $3, $3)`, old, old.Add(time.Hour), userID); err != nil {
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
	_, err = repository.CreateRun(ctx, operationsdomain.RetentionPolicy{ID: 1, Version: 1, DataClass: "delivery_attempts", RetentionDays: 1, Action: "delete", Enabled: true, Protected: true}, time.Now().UTC().Add(-24*time.Hour), 100, userID, time.Now().UTC())
	if !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("create protected delivery retention run error = %v, want invalid input", err)
	}
	var preserved int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM delivery_attempts WHERE delivery_id=$1`, deliveryID).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("protected delivery attempts = %d/%v, want 1", preserved, err)
	}
}
