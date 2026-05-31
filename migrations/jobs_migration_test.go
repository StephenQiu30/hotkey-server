package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestJobsMigrationDefinesAuditColumnsAndIdempotency(t *testing.T) {
	body, err := os.ReadFile("000005_jobs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"create table if not exists jobs",
		"job_type",
		"payload",
		"status",
		"attempt",
		"max_attempts",
		"idempotency_key",
		"last_error",
		"scheduled_at",
		"started_at",
		"finished_at",
		"created_at",
		"updated_at",
		"unique",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected migration to contain %q", want)
		}
	}
}
