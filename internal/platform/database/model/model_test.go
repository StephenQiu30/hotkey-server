package model

import "testing"

func TestSpecsHaveUniqueTablesAndColumns(t *testing.T) {
	seen := map[string]bool{}
	wantColumns := map[string][]string{
		"auth_sessions":               {"id", "user_id", "family_id", "absolute_expires_at", "revoked_at"},
		"auth_refresh_tokens":         {"id", "session_id", "token_hash", "expires_at", "used_at", "revoked_at"},
		"monitors":                    {"id", "version", "name", "status", "draft_config_version_id", "published_config_version_id", "deleted_at"},
		"monitor_config_versions":     {"id", "version", "monitor_id", "revision", "state", "config_hash", "published_at"},
		"monitor_rules":               {"id", "version", "config_version_id", "rule_type", "value"},
		"monitor_sources":             {"id", "version", "config_version_id", "source_connection_id", "query_signature"},
		"source_checkpoints":          {"id", "monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"},
		"collection_runs":             {"id", "source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "status", "updated_at"},
		"collection_run_targets":      {"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status", "updated_at"},
		"contents":                    {"id", "source_connection_id", "external_id", "dedupe_key", "dedupe_reason", "dedupe_version", "view_count", "like_count", "comment_count", "share_count", "deleted_at"},
		"collection_run_items":        {"id", "run_id", "source_connection_id", "source_code", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "outcome", "observed_at"},
		"collection_run_target_items": {"id", "collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"},
		"content_metric_snapshots":    {"id", "content_id", "captured_at", "view_count", "like_count", "comment_count", "share_count"},
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
	if got, want := len(seen), 52; got != want {
		t.Errorf("mapped table count = %d, want %d", got, want)
	}
}

func TestCollectionCaptureSpecsCoverDurableRunFacts(t *testing.T) {
	wantColumns := map[string][]string{
		"source_checkpoints":          {"id", "monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"},
		"collection_runs":             {"id", "source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "status", "updated_at"},
		"collection_run_targets":      {"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status", "updated_at"},
		"collection_run_items":        {"id", "run_id", "source_connection_id", "source_code", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "outcome", "observed_at"},
		"collection_run_target_items": {"id", "collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"},
	}
	gotColumns := make(map[string][]string, len(wantColumns))
	for _, spec := range All() {
		if _, wanted := wantColumns[spec.Table]; wanted {
			gotColumns[spec.Table] = spec.Columns
		}
	}
	for table, want := range wantColumns {
		got, found := gotColumns[table]
		if !found {
			t.Errorf("missing durable collection record %s", table)
			continue
		}
		if !sameColumns(got, want) {
			t.Errorf("durable collection columns for %s = %v, want %v", table, got, want)
		}
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
