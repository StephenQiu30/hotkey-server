package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGreenfieldMigrationsCoverDesignedTables(t *testing.T) {
	root := repositoryRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "db", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 7 {
		t.Fatalf("migration file count = %d, want 7", len(files))
	}

	var migrations strings.Builder
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(string(content))
		if !strings.Contains(text, "-- +goose up") || !strings.Contains(text, "-- +goose down") {
			t.Errorf("%s must contain Goose Up and Down sections", filepath.Base(file))
		}
		migrations.WriteString(text)
		migrations.WriteByte('\n')
	}

	all := migrations.String()
	for _, table := range append(businessTables(), operationalTables()...) {
		pattern := regexp.MustCompile(`create\s+table\s+(?:if\s+not\s+exists\s+)?` + regexp.QuoteMeta(table) + `\b`)
		if !pattern.MatchString(all) {
			t.Errorf("migration does not create table %s", table)
		}
	}
}

func TestGreenfieldMigrationsEnforceCriticalConstraints(t *testing.T) {
	all := readMigrationText(t)

	checks := map[string]string{
		"knowledge document has one target": "num_nonnulls(event_id, topic_id, report_id) = 1",
		"monitor score range":               "relevance_threshold between 0 and 100",
		"match score range":                 "final_score between 0 and 100",
		"monitor source idempotency":        "unique (monitor_id, source_connection_id)",
		"collection run idempotency":        "idempotency_key varchar(128) not null unique",
		"delivery idempotency":              "idempotency_key varchar(128) not null unique",
		"non-negative content metrics":      "view_count >= 0",
	}
	for name, snippet := range checks {
		if !strings.Contains(all, snippet) {
			t.Errorf("missing %s constraint: %q", name, snippet)
		}
	}
}

func TestApplicationDoesNotUseAutoMigrate(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, relative), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(strings.ToLower(string(content)), "automigrate") {
				t.Errorf("%s contains forbidden AutoMigrate call", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func readMigrationText(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "db", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var combined strings.Builder
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		combined.WriteString(strings.ToLower(string(content)))
		combined.WriteByte('\n')
	}
	return combined.String()
}

func businessTables() []string {
	return []string{
		"users", "user_preferences", "source_connections", "monitors", "monitor_rules",
		"monitor_sources", "source_authors", "contents", "content_assets", "monitor_matches",
		"events", "event_contents", "monitor_events", "entities", "entity_aliases",
		"event_entities", "event_claims", "claim_evidences", "topics", "topic_events",
		"topic_entities", "topic_relations", "entity_relations", "knowledge_documents",
		"knowledge_change_proposals", "knowledge_annotations", "reports", "report_items",
		"report_subscriptions", "ai_model_profiles", "retention_policies",
	}
}

func operationalTables() []string {
	return []string{
		"auth_sessions", "source_checkpoints", "collection_runs", "collection_run_items",
		"content_metric_snapshots", "event_metric_snapshots", "ai_runs", "ai_run_evidences",
		"content_embeddings", "monitor_embeddings", "event_embeddings", "topic_embeddings",
		"knowledge_revisions", "vault_sync_runs", "report_deliveries", "delivery_attempts",
		"audit_logs",
	}
}
