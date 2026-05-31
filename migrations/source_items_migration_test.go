package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestSourceItemsMigrationDefinesDedupeColumns(t *testing.T) {
	body, err := os.ReadFile("000006_source_items.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"create table if not exists source_items",
		"source_id",
		"title",
		"snippet",
		"raw_url",
		"canonical_url",
		"published_at",
		"content_hash",
		"language",
		"status",
		"duplicate_of_item_id",
		"unique",
		"status = 'primary' and duplicate_of_item_id is null",
		"status = 'duplicate' and duplicate_of_item_id is not null",
		"idx_source_items_content_hash",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected migration to contain %q", want)
		}
	}
}
