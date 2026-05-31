package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestReportsMigrationDefinesRequiredTables(t *testing.T) {
	body, err := os.ReadFile("000009_reports_ai_summaries.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS ai_summaries",
		"CREATE TABLE IF NOT EXISTS daily_reports",
		"prompt_version text NOT NULL",
		"input_hotspot_ids_json jsonb NOT NULL",
		"source_refs_json jsonb NOT NULL",
		"status text NOT NULL CHECK",
		"date date NOT NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	if strings.Contains(sql, "date text NOT NULL") {
		t.Fatalf("daily_reports.date must use SQL date, not text")
	}
}
