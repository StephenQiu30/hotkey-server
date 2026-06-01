package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestEmailDeliveriesMigrationDefinesDeliveryAuditTrail(t *testing.T) {
	body, err := os.ReadFile("000011_email_deliveries.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"create table if not exists email_deliveries",
		"recipient_user_id",
		"recipient_email",
		"report_id",
		"status",
		"attempt",
		"last_error",
		"sent_at",
		"created_at",
		"updated_at",
		"failed_config",
		"alter table users",
		"email_enabled",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected email deliveries migration to contain %q", want)
		}
	}
}
