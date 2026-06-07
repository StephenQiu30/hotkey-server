package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestEmailPreferencesMigrationAddsWeeklyColumns(t *testing.T) {
	body, err := os.ReadFile("000014_email_preferences.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"alter table users",
		"weekly_enabled",
		"boolean",
		"default false",
		"weekly_send_at",
		"text",
		"default '09:00'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected email preferences migration to contain %q", want)
		}
	}
}

func TestEmailPreferencesMigrationDownRemovesWeeklyColumns(t *testing.T) {
	body, err := os.ReadFile("000014_email_preferences.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"alter table users",
		"drop column",
		"weekly_send_at",
		"weekly_enabled",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected email preferences down migration to contain %q", want)
		}
	}
}
