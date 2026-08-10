package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestDocumentMatchSchemaUsesExactVersionedFactsAndAppendOnlyOverrides(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	for _, table := range []string{
		"relevance_evaluation_runs", "relevance_evaluation_slices", "relevance_decision_profiles", "document_match_decisions",
		"document_match_recall_signals", "document_match_overrides",
	} {
		if _, found := contract.Tables[table]; !found {
			t.Errorf("canonical catalog is missing %s", table)
		}
	}

	schema := strings.ToLower(canonicaldb.SchemaSQL)
	for name, snippet := range map[string]string{
		"profile lifecycle":            "status varchar(16) not null check (status in ('uncalibrated','shadow','active','rolled_back'))",
		"exact monitor version":        "monitor_version_id bigint not null",
		"exact document version":       "document_version_id bigint not null references document_versions(id)",
		"automatic decision identity":  "unique (monitor_version_id, document_version_id, matching_algorithm_version)",
		"normalized recall signals":    "create table if not exists document_match_recall_signals",
		"append-only automatic fact":   "create trigger document_match_decisions_append_only",
		"append-only override":         "create trigger document_match_overrides_append_only",
		"idempotent review":            "idempotency_key varchar(128) not null unique",
		"review fingerprint":           "command_fingerprint char(64) not null",
		"decision gate":                "create trigger document_match_decisions_integrity",
		"time-isolated evaluation":     "family_isolation_hash char(64) not null",
		"annotation protocol":          "annotation_protocol_version varchar(64) not null",
		"annotation guide digest":      "annotation_guideline_sha256 char(64) not null",
		"split strategy":               "split_strategy_version varchar(64) not null",
		"independent annotators":       "annotator_count smallint not null check (annotator_count between 2 and 20)",
		"annotation agreement":         "agreement_metric varchar(32) not null check (agreement_metric in ('cohen_kappa','krippendorff_alpha'))",
		"bounded evaluation window":    "sample_window_start timestamptz not null",
		"fitted calibration slope":     "calibration_slope numeric(12,7) not null",
		"fitted calibration intercept": "calibration_intercept numeric(12,7) not null",
		"evaluation append-only":       "create trigger relevance_evaluation_runs_append_only",
		"activation quality gate":      "relevance_decision_profiles_activation_gate",
		"override chain gate":          "create trigger document_match_overrides_integrity",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing %s contract: %q", name, snippet)
		}
	}

	for _, table := range []string{"relevance_evaluation_runs", "relevance_evaluation_slices", "relevance_decision_profiles", "document_match_decisions", "document_match_recall_signals", "document_match_overrides"} {
		block := documentMatchTableBlock(t, schema, table)
		for _, forbidden := range []string{"content_id", "truth", "credible", "credibility", "is_real", "confirmation", "corroborated", "unverified"} {
			if strings.Contains(block, forbidden) {
				t.Errorf("%s contains forbidden legacy/truth semantic %q", table, forbidden)
			}
		}
	}
}

func documentMatchTableBlock(t *testing.T, schema, table string) string {
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
