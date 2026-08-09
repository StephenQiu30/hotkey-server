package database

import (
	"context"
	"slices"
	"testing"
)

func TestMonitorIntentCanonicalUpgradeConvergesNonEmptyCatalogAndRepeats(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	const email = "monitor-intent-upgrade-preserved@example.test"
	if _, err := runtime.SQL.Exec(userInsertSQL, email, "preserved"); err != nil {
		t.Fatalf("insert pre-upgrade row: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
DO $$
DECLARE target record;
BEGIN
  FOR target IN
    SELECT tablename FROM pg_tables
    WHERE schemaname='public' AND tablename LIKE 'monitor_intent_%'
    ORDER BY tablename DESC
  LOOP
    EXECUTE format('DROP TABLE %I CASCADE', target.tablename);
  END LOOP;
END;
$$`); err != nil {
		t.Fatalf("prepare catalog without monitor intent persistence: %v", err)
	}

	inspection, err := InspectCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("InspectCanonicalUpgrade(): %v", err)
	}
	if !inspection.CanApply() || !slices.Contains(inspection.MissingTables, "monitor_intent_drafts") {
		t.Fatalf("upgrade inspection = %#v", inspection)
	}
	first, err := ApplyCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("ApplyCanonicalUpgrade(): %v", err)
	}
	assertUserCount(t, runtime, email, 1)
	if _, err := Verify(context.Background(), runtime.Pool); err != nil {
		t.Fatalf("Verify() after upgrade: %v", err)
	}
	second, err := ApplyCanonicalUpgrade(context.Background(), runtime.Pool, CanonicalUpgradeTarget)
	if err != nil {
		t.Fatalf("repeat ApplyCanonicalUpgrade(): %v", err)
	}
	if first.CatalogFingerprint != second.CatalogFingerprint {
		t.Fatalf("repeat upgrade fingerprint changed: %s / %s", first.CatalogFingerprint, second.CatalogFingerprint)
	}
}
