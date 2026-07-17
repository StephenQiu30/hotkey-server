package database

import "testing"

func TestCanonicalCatalogIncludesSetNullForeignKeyFromAlterTable(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract() error = %v", err)
	}
	checkpoint, found := contract.Tables["source_checkpoints"]
	if !found {
		t.Fatal("canonical catalog is missing source_checkpoints")
	}
	if got, want := checkpoint.Constraints.Foreign, 2; got != want {
		t.Fatalf("source_checkpoints foreign key count = %d, want %d", got, want)
	}
}

func TestCatalogConstraintNormalizationMatchesPostgreSQLLeaseCheck(t *testing.T) {
	expected := normalizeCatalogExpression("CHECK (status IN ('queued','running') AND lease_expires_at IS NOT NULL OR status IN ('succeeded') AND lease_expires_at IS NULL)")
	actual := normalizeCatalogExpression("CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text])) AND lease_expires_at IS NOT NULL OR status = ANY (ARRAY['succeeded'::text]) AND lease_expires_at IS NULL)")
	if actual != expected {
		t.Fatalf("normalized PostgreSQL lease CHECK = %q, want %q", actual, expected)
	}
}

func TestCatalogIndexNormalizationMatchesPostgreSQLInflightPredicate(t *testing.T) {
	expected := normalizeIndexDefinition("CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq ON ai_runs(reuse_key) WHERE status IN ('queued','running')")
	actual := normalizeIndexDefinition("CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq ON ai_runs USING btree (reuse_key) WHERE (status = ANY ((ARRAY['queued'::text, 'running'::text])))")
	if actual != expected {
		t.Fatalf("normalized PostgreSQL in-flight index = %q, want %q", actual, expected)
	}
}
