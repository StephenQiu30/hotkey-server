package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestRSSFeedsMigrationDefinesPrivateTokenBoundary(t *testing.T) {
	body, err := os.ReadFile("000010_rss_feeds.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS rss_feeds",
		"user_id text NOT NULL",
		"token_hash text NOT NULL",
		"enabled boolean NOT NULL DEFAULT true",
		"last_accessed_at timestamptz",
		"UNIQUE (token_hash)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	if strings.Contains(sql, " token text") {
		t.Fatal("rss_feeds must not store plaintext tokens")
	}
}
