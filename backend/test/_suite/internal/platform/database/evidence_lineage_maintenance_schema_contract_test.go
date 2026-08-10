package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestEvidenceLineageMaintenanceSchemaPersistsRunsAndAppendOnlyItems(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	for _, table := range []string{
		"evidence_lineage_migration_runs",
		"evidence_lineage_migration_items",
		"evidence_lineage_reconciliation_runs",
		"evidence_lineage_reconciliation_items",
	} {
		if _, found := contract.Tables[table]; !found {
			t.Errorf("canonical catalog is missing %s", table)
		}
	}

	schema := strings.ToLower(canonicaldb.SchemaSQL)
	for name, snippet := range map[string]string{
		"ordered phase vocabulary": "phase varchar(32) not null check (phase in ('source','evidence_metadata','document','match','family_event','evidence_state'))",
		"stable migration input":   "unique (run_id, legacy_resource_type, legacy_resource_id, input_sha256)",
		"resume checkpoint":        "last_legacy_resource_id bigint not null default 0",
		"bounded batch":            "batch_size integer not null check (batch_size between 1 and 1000)",
		"reconciliation scope":     "scope varchar(32) not null check (scope in ('pg-minio','pg-vault','rights-retention','all'))",
		"safe finding vocabulary": "finding varchar(32) not null check (finding in ('missing','orphan_within_grace','orphan_expired','digest_mismatch','policy_blocked','retention_blocked','active_pointer_invalid','healthy'))",
		"migration append-only":    "create trigger evidence_lineage_migration_items_append_only",
		"reconcile append-only":    "create trigger evidence_lineage_reconciliation_items_append_only",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing %s contract: %q", name, snippet)
		}
	}
}
