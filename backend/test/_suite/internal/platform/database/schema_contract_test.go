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

func TestNamedTableConstraintSignatureOmitsConstraintIdentifier(t *testing.T) {
	named := expectedConstraintDefinitions("CONSTRAINT reports_review_check CHECK (status <> 'published')")
	unnamed := expectedConstraintDefinitions("CHECK (status <> 'published')")
	if len(named) != 1 || len(unnamed) != 1 || named[0] != unnamed[0] {
		t.Fatalf("named constraint signature = %v, unnamed = %v", named, unnamed)
	}
}

func TestCatalogConstraintNormalizationMatchesPostgreSQLLeaseCheck(t *testing.T) {
	expected := normalizeCatalogExpression("CHECK (status IN ('queued','running') AND lease_expires_at IS NOT NULL OR status IN ('succeeded') AND lease_expires_at IS NULL)")
	actual := normalizeCatalogExpression("CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text])) AND lease_expires_at IS NOT NULL OR status = ANY (ARRAY['succeeded'::text]) AND lease_expires_at IS NULL)")
	if actual != expected {
		t.Fatalf("normalized PostgreSQL lease CHECK = %q, want %q", actual, expected)
	}
}

func TestCatalogConstraintNormalizationMatchesPostgreSQLFunctionBetweenCheck(t *testing.T) {
	expected := normalizeCatalogExpression("CHECK (octet_length(value) BETWEEN 1 AND 2048)")
	actual := normalizeCatalogExpression("CHECK (((octet_length(value) >= 1) AND (octet_length(value) <= 2048)))")
	if actual != expected {
		t.Fatalf("normalized PostgreSQL octet-length CHECK = %q, want %q", actual, expected)
	}
}

func TestCatalogIndexNormalizationMatchesPostgreSQLInflightPredicate(t *testing.T) {
	expected := normalizeIndexDefinition("CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq ON ai_runs(reuse_key) WHERE status IN ('queued','running')")
	actual := normalizeIndexDefinition("CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq ON ai_runs USING btree (reuse_key) WHERE (status = ANY ((ARRAY['queued'::text, 'running'::text])))")
	if actual != expected {
		t.Fatalf("normalized PostgreSQL in-flight index = %q, want %q", actual, expected)
	}
}

func TestCatalogIndexNormalizationMatchesLegacyVarcharArrayPredicate(t *testing.T) {
	expected := normalizeIndexDefinition("CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq ON ai_runs(reuse_key) WHERE status IN ('queued','running','validating','retry_wait')")
	actual := normalizeIndexDefinition("CREATE UNIQUE INDEX ai_runs_reuse_inflight_uq ON public.ai_runs USING btree (reuse_key) WHERE ((status)::text = ANY (ARRAY[('queued'::character varying)::text, ('running'::character varying)::text, ('validating'::character varying)::text, ('retry_wait'::character varying)::text]))")
	if actual != expected {
		t.Fatalf("normalized legacy PostgreSQL in-flight index = %q, want %q", actual, expected)
	}
}
