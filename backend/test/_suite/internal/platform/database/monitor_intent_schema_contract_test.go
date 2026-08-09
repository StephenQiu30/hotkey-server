package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestMonitorIntentSchemaKeepsDraftHistoryAndAnalysisFactsNormalized(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	wantTables := []string{
		"monitor_intent_drafts", "monitor_intent_draft_revisions", "monitor_intent_clauses",
		"monitor_intent_entities", "monitor_intent_entity_aliases", "monitor_intent_examples",
		"monitor_intent_analysis_runs", "monitor_intent_expansion_candidates",
		"monitor_intent_draft_candidates", "monitor_intent_mutation_receipts",
		"monitor_intent_preview_results", "monitor_intent_preview_samples",
		"monitor_intent_preview_recall_signals", "monitor_intent_preview_reasons",
		"monitor_intent_preview_warnings",
	}
	for _, table := range wantTables {
		if _, found := contract.Tables[table]; !found {
			t.Errorf("canonical catalog is missing %s", table)
		}
	}

	schema := strings.ToLower(canonicaldb.SchemaSQL)
	for name, snippet := range map[string]string{
		"independent immutable draft identity": "create table if not exists monitor_intent_drafts",
		"draft resource CAS":                   "resource_version bigint not null default 1",
		"exact historical revision":            "unique (draft_id, resource_version)",
		"orthogonal clause operator":           "operator varchar(16) not null check (operator in ('must','should','must_not'))",
		"orthogonal clause field":              "field varchar(24) not null check (field in ('term','phrase','action','location','language','region','source','time_window'))",
		"analysis run draft identity":          "draft_resource_version bigint not null",
		"analysis run kind":                    "kind varchar(16) not null check (kind in ('expansion','preview'))",
		"durable river ownership":              "river_job_id bigint not null unique references river_job(id)",
		"candidate provenance":                 "model_version varchar(128) not null",
		"candidate assessment":                 "similarity numeric(8,7) not null",
		"review receipt":                       "command_fingerprint char(64) not null",
		"normalized preview reasons":           "reason_type varchar(16) not null check (reason_type in ('match','exclusion'))",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing %s contract: %q", name, snippet)
		}
	}

	for _, table := range wantTables {
		block := monitorIntentTableBlock(t, schema, table)
		for _, forbidden := range []string{"body text", "body_text", "raw_bytes", "raw_payload", "content_markdown"} {
			if strings.Contains(block, forbidden) {
				t.Errorf("%s contains forbidden body/raw fact %q", table, forbidden)
			}
		}
		if table != "monitor_intent_analysis_runs" && strings.Contains(block, "jsonb") {
			t.Errorf("%s uses a generic JSON persistence bucket", table)
		}
	}
	for _, legacyTable := range []string{"monitor_rules", "monitor_sources"} {
		block := monitorIntentTableBlock(t, schema, legacyTable)
		if strings.Contains(block, "intent_") || strings.Contains(block, "objective") {
			t.Errorf("legacy %s is coupled to monitor intent persistence", legacyTable)
		}
	}
}

func monitorIntentTableBlock(t *testing.T, schema, table string) string {
	t.Helper()
	start := strings.Index(schema, "create table if not exists "+table+" (")
	if start < 0 {
		t.Fatalf("missing table %s", table)
	}
	end := strings.Index(schema[start:], "\n);")
	if end < 0 {
		t.Fatalf("unterminated table %s", table)
	}
	return schema[start : start+end]
}
