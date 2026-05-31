package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestHotspotScoresMigrationDefinesSchema(t *testing.T) {
	body, err := os.ReadFile("000008_hotspot_scores.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"create table if not exists hotspot_scores",
		"cluster_id",
		"total_score",
		"source_count_score",
		"freshness_score",
		"relevance_score",
		"propagation_score",
		"quality_score",
		"explanation",
		"score_version",
		"created_at",
		"updated_at",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected migration to contain %q", want)
		}
	}
}
