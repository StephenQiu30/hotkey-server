package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database/model"
)

func TestCompleteSchemaCoversMappedRecords(t *testing.T) {
	schema := readSchemaText(t)
	for _, spec := range model.All() {
		create := regexp.MustCompile(`(?s)create\s+table\s+if\s+not\s+exists\s+` + regexp.QuoteMeta(spec.Table) + `\s*\(`)
		if !create.MatchString(schema) {
			t.Errorf("complete schema does not create %s", spec.Table)
			continue
		}
		block := tableBlock(t, schema, spec.Table)
		for _, column := range spec.Columns {
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(column) + `\b`).MatchString(block) {
				t.Errorf("schema does not contain mapped column %s.%s", spec.Table, column)
			}
		}
	}
	for table, columns := range map[string][]string{
		"auth_sessions":               {"family_id", "absolute_expires_at", "revoked_at"},
		"auth_refresh_tokens":         {"session_id", "token_hash", "expires_at", "used_at", "revoked_at"},
		"monitors":                    {"draft_config_version_id", "published_config_version_id"},
		"monitor_config_versions":     {"monitor_id", "revision", "state", "config_hash", "published_at"},
		"monitor_rules":               {"config_version_id"},
		"monitor_sources":             {"config_version_id", "query_signature"},
		"source_checkpoints":          {"monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"},
		"collection_runs":             {"source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "updated_at"},
		"collection_run_targets":      {"collection_run_id", "monitor_source_id", "monitor_config_version_id", "updated_at"},
		"collection_run_items":        {"run_id", "source_connection_id", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "observed_at"},
		"collection_run_target_items": {"collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"},
	} {
		block := tableBlock(t, schema, table)
		for _, column := range columns {
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(column) + `\b`).MatchString(block) {
				t.Errorf("schema does not contain required authentication column %s.%s", table, column)
			}
		}
	}
}

func tableBlock(t *testing.T, schema, table string) string {
	t.Helper()
	start := strings.Index(schema, "create table if not exists "+table+" (")
	if start < 0 {
		t.Fatalf("missing table block for %s", table)
	}
	end := strings.Index(schema[start:], "\n);")
	if end < 0 {
		t.Fatalf("unterminated table block for %s", table)
	}
	return schema[start : start+end+3]
}

func TestSchemaHasNoSecondFactSource(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := os.Stat(filepath.Join(root, "db", "schema")); err == nil {
		t.Error("legacy split schema directory db/schema must not exist")
	}
	if _, err := os.Stat(filepath.Join(root, "db", "migrations")); err == nil {
		t.Error("parallel migration directory db/migrations must not exist")
	}
}

func TestGreenfieldSchemaEnforcesCriticalConstraints(t *testing.T) {
	schema := readSchemaText(t)
	checks := map[string]string{
		"knowledge document has one target":          "num_nonnulls(event_id, topic_id, report_id) = 1",
		"monitor config draft uniqueness":            "where state = 'draft'",
		"monitor config published uniqueness":        "where state = 'published'",
		"monitor active name uniqueness":             "where deleted_at is null and status <> 'archived'",
		"monitor config published timestamp":         "published_at timestamptz",
		"monitor config historical key":              "unique (id, monitor_id)",
		"monitor match historical key":               "foreign key (monitor_config_version_id, monitor_id) references monitor_config_versions(id, monitor_id)",
		"monitor source historical key":              "unique (id, config_version_id)",
		"run target historical key":                  "foreign key (monitor_source_id, monitor_config_version_id) references monitor_sources(id, config_version_id)",
		"published config immutability trigger":      "create trigger monitor_config_versions_immutable",
		"published rule immutability trigger":        "create trigger monitor_rules_immutable",
		"published source immutability trigger":      "create trigger monitor_sources_immutable",
		"source semantic immutability trigger":       "create trigger source_connections_semantic_immutable",
		"source query signature":                     "query_signature char(64)",
		"match score range":                          "final_score between 0 and 100",
		"monitor source idempotency":                 "unique (config_version_id, source_connection_id)",
		"shared collection run idempotency":          "unique (source_connection_id, query_signature, window_start, window_end)",
		"collection item capture payload":            "captured_item jsonb not null",
		"collection item payload disposition":        "raw_payload_disposition varchar(32) not null check (raw_payload_disposition in ('discarded', 'captured_item_only'))",
		"collection target item reconciliation":      "unique (collection_run_target_id, collection_run_item_id)",
		"collection target run alignment key":        "unique (id, collection_run_id)",
		"collection item run alignment key":          "unique (id, run_id)",
		"collection target item target run key":      "foreign key (collection_run_target_id, collection_run_id) references collection_run_targets(id, collection_run_id) on delete cascade",
		"collection target item item run key":        "foreign key (collection_run_item_id, collection_run_id) references collection_run_items(id, run_id) on delete cascade",
		"checkpoint successful run foreign key":      "foreign key (last_successful_run_id) references collection_runs(id) on delete set null",
		"delivery idempotency":                       "idempotency_key varchar(128) not null unique",
		"nullable content metrics":                   "view_count bigint check (view_count >= 0)",
		"nullable content metric snapshots":          "captured_at timestamptz not null, view_count bigint check (view_count >= 0)",
		"content duplicate evidence":                 "dedupe_reason varchar(32), dedupe_version varchar(64)",
		"collection run source alignment key":        "unique (id, source_connection_id)",
		"collection item source ownership":           "source_connection_id bigint not null",
		"collection item run source foreign key":     "foreign key (run_id, source_connection_id) references collection_runs(id, source_connection_id) on delete cascade",
		"collection item content source foreign key": "foreign key (content_id, source_connection_id) references contents(id, source_connection_id)",
		"collection item ingestion state":            "outcome = 'captured' and (",
		"vector extension":                           "create extension if not exists vector",
		"fixed embedding dimension":                  "halfvec(1024)",
	}
	for name, snippet := range checks {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing %s constraint: %q", name, snippet)
		}
	}
}

func TestSchemaHasVersionPointerForeignKeys(t *testing.T) {
	schema := readSchemaText(t)
	for name, column := range map[string]string{
		"monitors_draft_config_version_fkey":     "draft_config_version_id",
		"monitors_published_config_version_fkey": "published_config_version_id",
	} {
		pattern := regexp.MustCompile(`(?s)add\s+constraint\s+` + regexp.QuoteMeta(name) + `\s+foreign\s+key\s*\(` + regexp.QuoteMeta(column) + `\)\s+references\s+monitor_config_versions\s*\(id\)\s+on\s+delete\s+restrict`)
		if !pattern.MatchString(schema) {
			t.Errorf("schema does not define %s as ON DELETE RESTRICT monitor configuration foreign key", name)
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

func TestSchemaIsIdempotentWhenTestDatabaseIsConfigured(t *testing.T) {
	dsn := os.Getenv("HOTKEY_TEST_DSN")
	if dsn == "" {
		t.Skip("HOTKEY_TEST_DSN is not configured")
	}
	root := repositoryRoot(t)
	for run := 1; run <= 2; run++ {
		command := exec.Command("psql", dsn, "-v", "ON_ERROR_STOP=1", "-f", filepath.Join(root, "db", "schema.sql"))
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("schema run %d failed: %v\n%s", run, err, output)
		}
	}
}

func readSchemaText(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(string(content))
}
