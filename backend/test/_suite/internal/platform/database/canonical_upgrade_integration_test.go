package database

import (
	"context"
	"slices"
	"testing"
)

func TestCanonicalUpgradeDryRunApplyAndRepeatPreserveRows(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	const email = "canonical-upgrade-preserved@example.test"
	if _, err := runtime.SQL.Exec(userInsertSQL, email, "preserved"); err != nil {
		t.Fatalf("insert pre-upgrade row: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
DROP TABLE derived_artifacts, document_versions, documents,
  source_observation_evidences, source_observations, evidence_snapshots,
  source_rights_decisions, source_rights_decision_batches, source_rights_policies CASCADE`); err != nil {
		t.Fatalf("prepare catalog without evidence lineage: %v", err)
	}

	inspection, err := InspectCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("InspectCanonicalUpgrade(): %v", err)
	}
	if !inspection.CanApply() || !slices.Contains(inspection.MissingTables, "document_versions") || slices.Contains(inspection.CurrentTables, "document_versions") {
		t.Fatalf("dry-run inspection = %#v", inspection)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("catalog without evidence lineage unexpectedly passed target verification")
	}

	first, err := ApplyCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("ApplyCanonicalUpgrade(): %v", err)
	}
	assertUserCount(t, runtime, email, 1)
	second, err := ApplyCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("repeat ApplyCanonicalUpgrade(): %v", err)
	}
	if first.CatalogFingerprint != second.CatalogFingerprint {
		t.Fatalf("repeat upgrade fingerprint changed: %s / %s", first.CatalogFingerprint, second.CatalogFingerprint)
	}
}

func TestCanonicalUpgradeRejectsEmptyAndUnknownTargets(t *testing.T) {
	runtime := openEmptyTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	inspection, err := InspectCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("InspectCanonicalUpgrade(empty): %v", err)
	}
	if inspection.CanApply() || !slices.Contains(inspection.Blockers, "public_schema_empty_use_db_init") {
		t.Fatalf("empty inspection blockers = %v", inspection.Blockers)
	}
	if _, err := ApplyCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget); err == nil {
		t.Fatal("empty schema canonical upgrade succeeded")
	}
	if _, err := InspectCanonicalUpgrade(context.Background(), runtime.Pool, "999"); err == nil {
		t.Fatal("unknown canonical upgrade target was accepted")
	}
}
