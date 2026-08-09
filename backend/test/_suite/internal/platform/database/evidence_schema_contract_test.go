package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestEvidenceSchemaSeparatesRawObservationDocumentAndProjectionFacts(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	for _, table := range []string{
		"source_rights_policies", "source_rights_decisions", "evidence_snapshots",
		"source_observations", "source_observation_evidences", "documents",
		"document_versions", "derived_artifacts",
	} {
		if _, found := contract.Tables[table]; !found {
			t.Errorf("canonical catalog is missing %s", table)
		}
	}

	schema := strings.ToLower(canonicaldb.SchemaSQL)
	for name, snippet := range map[string]string{
		"single-action rights decision":     "action varchar(32) not null check (action in ('fetch','store_raw','store_derived','display_private','redistribute','quote','embed_local','send_external_model','retain'))",
		"decision policy snapshot":          "policy_scope_type varchar(32) not null",
		"decision effective lifetime":       "effective_from timestamptz not null",
		"decision supersession":             "supersedes_decision_id bigint",
		"retain-only duration":              "action = 'retain' and retention_days between 1 and 3650 or action <> 'retain' and retention_days is null",
		"policy binding trigger":            "create trigger source_rights_decisions_policy_binding",
		"current exact allow resolver":      "create or replace function current_rights_action_allowed",
		"current terminal action resolver":  "create or replace function current_rights_action_is_allowed",
		"conservative retention resolver":   "create or replace function current_rights_retention_days",
		"raw exact rights binding":          "create trigger evidence_snapshots_rights_binding",
		"raw retain decision":               "retain_rights_decision_id bigint not null",
		"raw collector profile identity":    "collector_profile_version varchar(64) not null",
		"raw collector profile canonical":   "collector_profile_version ~ '^[a-z0-9][a-z0-9._:-]{0,63}$'",
		"raw capture clock boundary":        "captured_at <= (created_at + '00:05:00'::interval)",
		"source-scoped snapshot identity":   "unique (source_connection_id, snapshot_key)",
		"raw response header allowlist":     "response_headers - array['content-type','etag','last-modified','date','link','retry-after']::text[] = '{}'::jsonb",
		"observation snapshot many-to-many": "create table if not exists source_observation_evidences",
		"immutable document content":        "create trigger document_versions_lifecycle",
		"document quality is nullable":      "quality_score numeric(5,2) check (quality_score is null or quality_score between 0 and 100)",
		"document profile version identity": "unique (document_id, source_observation_id, content_sha256, extractor_profile_version)",
		"readable rights receipt":           "display_private_rights_decision_id bigint",
		"readable prerequisite check":       "document_version_readable",
		"derived exact rights binding":      "create trigger derived_artifacts_rights_binding",
		"transformer artifact identity":     "unique (document_version_id, artifact_type, transformer_profile_sha256)",
		"one active projection":             "derived_artifacts_one_active_per_type_uq",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing %s contract: %q", name, snippet)
		}
	}

	for name, snippet := range map[string]string{
		"policy hash":                   "policy_hash ~ '^[0-9a-f]{64}$'",
		"observation upstream hash":     "upstream_identity ~ '^[0-9a-f]{64}$'",
		"document key":                  "document_key ~ '^[0-9a-f]{64}$'",
		"document version key":          "version_key ~ '^[0-9a-f]{64}$'",
		"document content hash":         "content_sha256 ~ '^[0-9a-f]{64}$'",
		"extractor profile hash":        "extractor_profile_sha256 ~ '^[0-9a-f]{64}$'",
		"transformer profile hash":      "transformer_profile_sha256 ~ '^[0-9a-f]{64}$'",
		"derived artifact payload hash": "sha256 ~ '^[0-9a-f]{64}$'",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing lowercase hexadecimal %s contract: %q", name, snippet)
		}
	}

	policyBlock := evidenceTableBlock(t, schema, "source_rights_policies")
	for _, forbidden := range []string{
		"fetch_decision", "store_raw_decision", "store_derived_decision", "display_private_decision",
		"redistribute_decision", "quote_decision", "embed_local_decision",
		"send_external_model_decision", "retain_decision", "retention_days",
	} {
		if strings.Contains(policyBlock, forbidden) {
			t.Errorf("source_rights_policies contains drifting action state %q", forbidden)
		}
	}

	observationBlock := evidenceTableBlock(t, schema, "source_observations")
	if strings.Contains(observationBlock, "selected_payload_sha256") {
		t.Error("source_observations contains evidence-specific selected payload hash")
	}
	locatorBlock := evidenceTableBlock(t, schema, "source_observation_evidences")
	for _, required := range []string{
		"selected_payload_sha256 char(64) not null",
		"selected_payload_sha256 ~ '^[0-9a-f]{64}$'",
	} {
		if !strings.Contains(locatorBlock, required) {
			t.Errorf("source_observation_evidences is missing locator hash contract %q", required)
		}
	}
}

func TestEvidenceFactsDoNotIntroduceTruthOrCredibilitySemantics(t *testing.T) {
	schema := strings.ToLower(canonicaldb.SchemaSQL)
	for _, table := range []string{"source_rights_policies", "source_rights_decisions", "evidence_snapshots", "source_observations", "source_observation_evidences", "documents", "document_versions", "derived_artifacts"} {
		block := evidenceTableBlock(t, schema, table)
		for _, forbidden := range []string{"truth_score", "is_real", "credibility", "confirmation_score"} {
			if strings.Contains(block, forbidden) {
				t.Errorf("%s contains forbidden truth-like field %q", table, forbidden)
			}
		}
	}
}

func evidenceTableBlock(t *testing.T, schema, table string) string {
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
