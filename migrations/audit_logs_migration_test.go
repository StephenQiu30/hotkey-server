package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestAuditLogsMigrationDefinesAdminAuditTrail(t *testing.T) {
	body, err := os.ReadFile("000012_audit_logs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"create table if not exists audit_logs",
		"actor_id",
		"action",
		"resource_type",
		"resource_id",
		"result",
		"created_at",
		"idx_audit_logs_actor_created_at",
		"idx_audit_logs_resource_created_at",
		"idx_audit_logs_created_at",
		"check (action in ('create', 'update', 'delete'))",
		"check (result in ('success', 'failure'))",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected audit log migration to contain %q", want)
		}
	}
}
