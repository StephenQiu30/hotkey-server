package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestReportTypeMigrationAddsColumnsAndSkippedStatus(t *testing.T) {
	body, err := os.ReadFile("000012_report_type_and_skipped_status.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"report_type text NOT NULL DEFAULT 'daily'",
		"daily_report_ids_json jsonb NOT NULL DEFAULT",
		"'skipped'",
		"email_deliveries_status_check",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestReportTypeMigrationDownRemovesColumnsAndRevertsConstraint(t *testing.T) {
	body, err := os.ReadFile("000012_report_type_and_skipped_status.down.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"DROP COLUMN IF EXISTS report_type",
		"DROP COLUMN IF EXISTS daily_report_ids_json",
		"email_deliveries_status_check",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("down migration missing %q", want)
		}
	}
	// Verify 'skipped' is NOT in the reverted CHECK constraint definition
	if strings.Contains(sql, "CHECK (status IN") {
		checkStart := strings.Index(sql, "CHECK (status IN")
		checkEnd := strings.Index(sql[checkStart:], ")")
		checkClause := sql[checkStart : checkStart+checkEnd+1]
		if strings.Contains(checkClause, "'skipped'") {
			t.Fatal("down migration reverted CHECK should not contain 'skipped'")
		}
	}
}
