package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddingsHotspotsMigrationDefinesPgvectorSchema(t *testing.T) {
	body, err := os.ReadFile("000007_embeddings_hotspots.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))

	for _, want := range []string{
		"create extension if not exists vector",
		"create table if not exists item_embeddings",
		"embedding vector",
		"status",
		"failed_config",
		"create table if not exists hotspot_clusters",
		"centroid vector",
		"create table if not exists hotspot_items",
		"source_items",
		"idx_item_embeddings_embedding",
		"idx_hotspot_items_item_id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected migration to contain %q", want)
		}
	}
}
