package model

import "testing"

func TestSpecsHaveUniqueTablesAndColumns(t *testing.T) {
	seen := map[string]bool{}
	wantColumns := map[string][]string{
		"auth_sessions":           {"id", "user_id", "family_id", "absolute_expires_at", "revoked_at"},
		"auth_refresh_tokens":     {"id", "session_id", "token_hash", "expires_at", "used_at", "revoked_at"},
		"monitors":                {"id", "version", "name", "status", "draft_config_version_id", "published_config_version_id", "deleted_at"},
		"monitor_config_versions": {"id", "version", "monitor_id", "revision", "state", "config_hash", "published_at"},
		"monitor_rules":           {"id", "version", "config_version_id", "rule_type", "value"},
		"monitor_sources":         {"id", "version", "config_version_id", "source_connection_id", "query_signature"},
		"collection_runs":         {"id", "source_connection_id", "query_signature", "window_start", "window_end", "status"},
		"collection_run_targets":  {"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status"},
	}
	for _, spec := range All() {
		if spec.Table == "" || seen[spec.Table] {
			t.Fatalf("invalid or duplicate table spec %q", spec.Table)
		}
		seen[spec.Table] = true
		if len(spec.Columns) == 0 {
			t.Fatalf("%s has no mapped columns", spec.Table)
		}
		if want, ok := wantColumns[spec.Table]; ok && !sameColumns(spec.Columns, want) {
			t.Errorf("mapped columns for %s = %v, want %v", spec.Table, spec.Columns, want)
		}
	}
	for table := range wantColumns {
		if !seen[table] {
			t.Errorf("missing mapped table %s", table)
		}
	}
	if got, want := len(seen), 51; got != want {
		t.Errorf("mapped table count = %d, want %d", got, want)
	}
}

func sameColumns(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func TestPersistenceMetadataMakesEveryBusinessTableVersioned(t *testing.T) {
	for _, spec := range All() {
		metadata, found := PersistenceFor(spec.Table)
		if !found {
			t.Fatalf("PersistenceFor(%q) did not return metadata", spec.Table)
		}
		if spec.Lifecycle == LifecycleBusiness && metadata.VersionColumn != "version" {
			t.Errorf("business table %s VersionColumn = %q, want version", spec.Table, metadata.VersionColumn)
		}
		if spec.Lifecycle == LifecycleOperational && metadata.VersionColumn != "" {
			t.Errorf("operational table %s VersionColumn = %q, want empty", spec.Table, metadata.VersionColumn)
		}
	}
}
