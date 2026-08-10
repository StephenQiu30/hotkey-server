package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestContentFamilySchemaStoresRightsBoundFingerprintsAndImmutableLineageWithoutBody(t *testing.T) {
	schema := strings.ToLower(canonicaldb.SchemaSQL)
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	for _, table := range []string{"content_fingerprints", "content_families", "content_lineage_decisions", "content_family_members", "content_lineage_feedbacks"} {
		if _, found := contract.Tables[table]; !found {
			t.Fatalf("canonical catalog is missing %s", table)
		}
		block := documentMatchTableBlock(t, schema, table)
		for _, forbidden := range []string{" body ", " plaintext ", " markdown ", " credibility", " truth", " confidence"} {
			if strings.Contains(block, forbidden) {
				t.Fatalf("%s contains forbidden public/body semantic %q", table, forbidden)
			}
		}
	}
	fingerprint := documentMatchTableBlock(t, schema, "content_fingerprints")
	for _, required := range []string{
		"derived_artifact_id", "store_derived_rights_decision_id", "retain_rights_decision_id",
		"normalized_content_sha256", "simhash_hex", "minhash", "retention_until", "lifecycle_state",
		"check (octet_length(minhash) = 512)", "content_fingerprints_exact_rights",
	} {
		if !strings.Contains(fingerprint+schema, required) {
			t.Fatalf("content fingerprint schema missing %q", required)
		}
	}
	decision := documentMatchTableBlock(t, schema, "content_lineage_decisions")
	for _, required := range []string{"exact_copy", "near_duplicate", "syndicated_from", "translation_of", "revision_of", "unrelated", "idempotency_key", "command_fingerprint", "result_family_version"} {
		if !strings.Contains(decision, required) {
			t.Fatalf("content lineage decision schema missing %q", required)
		}
	}
	for _, required := range []string{
		"create trigger content_fingerprints_integrity", "create trigger content_fingerprints_lifecycle",
		"create trigger content_lineage_decisions_append_only", "create trigger content_lineage_feedbacks_append_only",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("content lineage schema missing %q", required)
		}
	}
}
