//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestRepositoryProjectsOldestUnknownDeliveryWithoutSensitiveProviderFacts(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, _, firstNotificationID := insertEmailNotificationFixture(t, runtime, now.Add(-time.Minute), true)
	_, _, secondNotificationID := insertEmailNotificationFixture(t, runtime, now, true)
	const secretCanary = "delivery-secret-must-not-leak"
	var firstAttemptID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_delivery_attempts(
user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,
provider_message_id,error_code,attempted_at)
VALUES ($1,'email',$2,1,'unknown',$3,1,$4,'provider_outcome_unconfirmed',$5)
RETURNING id`, firstNotificationID, application.PrimaryEmailDeliveryTarget, strings.Repeat("a", 64),
		secretCanary, now.Add(-time.Minute)).Scan(&firstAttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO notification_delivery_attempts(
user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,
provider_message_id,error_code,attempted_at)
VALUES ($1,'email',$2,1,'unknown',$3,1,$4,'provider_receipt_unavailable',$5)`,
		secondNotificationID, application.PrimaryEmailDeliveryTarget, strings.Repeat("b", 64),
		secretCanary, now); err != nil {
		t.Fatal(err)
	}

	summary, found, err := NewRepository(runtime).UnknownDeliveryAlert(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found || summary.AttemptID != firstAttemptID || summary.NotificationID != firstNotificationID ||
		summary.ResourceType != "micro_event" || summary.ResourceID <= 0 || summary.AffectedCount != 2 ||
		!summary.TriggeredAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("unknown delivery summary = %#v / found=%t", summary, found)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretCanary) || strings.Contains(string(encoded), "dispatch_key") ||
		strings.Contains(string(encoded), "provider_message") || strings.Contains(string(encoded), "error_code") {
		t.Fatalf("unknown delivery summary leaked provider facts: %s", encoded)
	}

	claimed, err := NewRepository(runtime).ClaimNextEmailDelivery(ctx, application.ClaimNextEmailDeliveryCommand{
		ClaimToken: strings.Repeat("c", 64), LeaseDuration: time.Minute,
	})
	if err != nil || claimed.Claimed {
		t.Fatalf("unknown delivery was replayed by alert read = %#v / %v", claimed, err)
	}
}

func TestRepositoryReturnsNoUnknownDeliveryAlertForKnownOutcomes(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	summary, found, err := NewRepository(runtime).UnknownDeliveryAlert(ctx)
	if err != nil || found || summary.AttemptID != 0 {
		t.Fatalf("empty unknown delivery summary = %#v / found=%t / %v", summary, found, err)
	}
}
