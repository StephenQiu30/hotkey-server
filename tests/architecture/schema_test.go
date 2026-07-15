package architecture_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPerTableSchemaFilesCoverDesignedTables(t *testing.T) {
	root := repositoryRoot(t)
	schemaRoot := filepath.Join(root, "db", "schema")
	for _, table := range append(businessTables(), operationalTables()...) {
		file := filepath.Join(schemaRoot, table+".sql")
		content, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("read schema for %s: %v", table, err)
			continue
		}
		text := strings.ToLower(string(content))
		pattern := regexp.MustCompile(`create\s+table\s+(?:if\s+not\s+exists\s+)?` + regexp.QuoteMeta(table) + `\b`)
		if !pattern.MatchString(text) {
			t.Errorf("%s does not create table %s", filepath.Base(file), table)
		}
	}
}

func TestSchemaManifestContainsEveryTableOnce(t *testing.T) {
	root := repositoryRoot(t)
	file, err := os.Open(filepath.Join(root, "db", "schema", "manifest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	want := append(businessTables(), operationalTables()...)
	wantSet := make(map[string]bool, len(want))
	for _, table := range want {
		wantSet[table+".sql"] = true
	}
	seen := make(map[string]int, len(want))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" || strings.HasPrefix(entry, "#") || entry == "extensions.sql" || entry == "indexes.sql" {
			continue
		}
		seen[entry]++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for entry := range wantSet {
		if seen[entry] != 1 {
			t.Errorf("manifest entry %s count = %d, want 1", entry, seen[entry])
		}
	}
}

func TestGreenfieldSchemaEnforcesCriticalConstraints(t *testing.T) {
	all := readSchemaText(t)

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

func readSchemaText(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "db", "schema", "*.sql"))
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
