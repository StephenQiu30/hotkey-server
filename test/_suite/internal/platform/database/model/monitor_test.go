package model

import "testing"

func TestVersionedMonitorRecordsExposeHistoricalKeys(t *testing.T) {
	var monitor Monitor
	var config MonitorConfigVersion
	var rule MonitorRule
	var source MonitorSource
	var run CollectionRun
	var target CollectionRunTarget

	if monitor.DraftConfigVersionID != nil || monitor.PublishedConfigVersionID != nil {
		t.Fatal("zero-value monitor configuration pointers must be optional")
	}
	if config.MonitorID != 0 || config.Revision != 0 || config.PublishedAt != nil {
		t.Fatal("zero-value configuration version does not expose versioned monitor fields")
	}
	if rule.ConfigVersionID != 0 || source.ConfigVersionID != 0 || source.QuerySignature != "" {
		t.Fatal("rule/source records must belong to a configuration version")
	}
	if run.SourceConnectionID != 0 || run.QuerySignature != "" || !run.WindowStart.IsZero() || !run.WindowEnd.IsZero() {
		t.Fatal("collection run must expose the shared source/query/window key")
	}
	if target.CollectionRunID != 0 || target.MonitorSourceID != 0 || target.MonitorConfigVersionID != 0 {
		t.Fatal("collection run target must preserve immutable monitor source ownership")
	}
}

func TestVersionedMonitorPersistenceIsConservative(t *testing.T) {
	for _, table := range []string{"monitor_config_versions", "monitor_rules", "monitor_sources"} {
		metadata, ok := PersistenceFor(table)
		if !ok {
			t.Fatalf("PersistenceFor(%q) missing", table)
		}
		if metadata.Deletion != DeletionRetained {
			t.Errorf("%s deletion policy = %q, want retained historical records", table, metadata.Deletion)
		}
	}
	metadata, ok := PersistenceFor("monitor_config_versions")
	if !ok || !sameColumns(metadata.AllowedSort, []string{"revision", "id"}) || !sameColumns(metadata.CursorFields, []string{"revision", "id"}) {
		t.Errorf("monitor configuration persistence metadata = %#v, want revision cursor", metadata)
	}
}
