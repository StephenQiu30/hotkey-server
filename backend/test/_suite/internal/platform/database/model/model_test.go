package model

import "testing"

func TestSpecsHaveUniqueTablesAndColumns(t *testing.T) {
	seen := map[string]bool{}
	wantColumns := map[string][]string{
		"ai_model_profiles":            {"id", "version", "name", "task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions", "timeout_seconds", "max_attempts", "max_cost", "daily_budget", "fallback_priority", "enabled", "deleted_at"},
		"ai_runs":                      {"id", "task_type", "target_type", "target_id", "model_profile_id", "model_profile_version", "model_version", "prompt_version", "input_schema_version", "schema_version", "parameters_version", "input_hash", "evidence_set_hash", "reuse_key", "attempt", "max_attempts", "repair_attempted", "retry_after", "error_code", "budget_day", "reserved_cost", "lease_expires_at", "status"},
		"ai_budget_ledgers":            {"id", "model_profile_id", "budget_day", "reserved_cost", "settled_cost", "overage_blocked", "updated_at"},
		"quota_usage_ledgers":          {"id", "dimension", "subject_type", "subject_id", "window_start", "window_end", "used", "updated_at"},
		"content_embeddings":           {"id", "content_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"},
		"monitor_embeddings":           {"id", "monitor_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "query_text", "embedding", "active"},
		"event_embeddings":             {"id", "event_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"},
		"topic_embeddings":             {"id", "topic_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"},
		"auth_sessions":                {"id", "user_id", "family_id", "absolute_expires_at", "revoked_at"},
		"auth_refresh_tokens":          {"id", "session_id", "token_hash", "expires_at", "used_at", "revoked_at"},
		"monitors":                     {"id", "version", "name", "status", "draft_config_version_id", "published_config_version_id", "deleted_at"},
		"monitor_config_versions":      {"id", "version", "monitor_id", "revision", "state", "config_hash", "published_at"},
		"monitor_rules":                {"id", "version", "config_version_id", "rule_type", "value"},
		"monitor_sources":              {"id", "version", "config_version_id", "source_connection_id", "query_signature"},
		"source_checkpoints":           {"id", "monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"},
		"collection_runs":              {"id", "source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "status", "updated_at"},
		"collection_run_targets":       {"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status", "updated_at"},
		"contents":                     {"id", "source_connection_id", "external_id", "dedupe_key", "dedupe_reason", "dedupe_version", "view_count", "like_count", "comment_count", "share_count", "deleted_at"},
		"monitor_matches":              {"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "input_hash", "scoring_version", "final_score", "decision", "decision_origin", "embedding_model_profile_id", "embedding_model_profile_version", "embedding_model_version", "review_ai_run_id"},
		"monitor_match_feedbacks":      {"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "monitor_match_id", "actor_user_id", "feedback_type"},
		"monitor_feedback_suggestions": {"id", "version", "monitor_id", "monitor_config_version_id", "suggestion_type", "value", "support_count", "status", "reviewed_by_user_id"},
		"collection_run_items":         {"id", "run_id", "source_connection_id", "source_code", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "outcome", "observed_at"},
		"collection_run_target_items":  {"id", "collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"},
		"content_metric_snapshots":     {"id", "content_id", "captured_at", "view_count", "like_count", "comment_count", "share_count"},
		"metric_capability_profiles":   {"id", "version", "source_type", "profile_version", "status", "published_at", "archived_at"},
		"event_updates":                {"id", "version", "event_id", "sequence_no", "kind", "summary", "observed_at", "reason_codes", "before_state", "after_state", "evidence_set_hash", "idempotency_key", "created_at"},
		"alert_threads":                {"id", "version", "monitor_id", "monitor_config_version_id", "monitor_revision", "monitor_config_hash", "event_id", "trigger_type", "policy_version", "state", "severity", "event_threshold_snapshot", "alert_min_heat_snapshot", "alert_min_momentum_snapshot", "alert_min_breadth_snapshot", "alert_warning_threshold_snapshot", "alert_critical_threshold_snapshot", "alert_cooldown_minutes_snapshot", "title_snapshot", "reason_snapshot", "first_triggered_at", "last_triggered_at", "occurrence_count", "cooldown_until", "acknowledged_at", "acknowledged_by_user_id", "resolved_at", "resolved_by_user_id", "suppressed_at", "suppressed_by_user_id", "created_at", "updated_at"},
		"alert_occurrences":            {"id", "alert_thread_id", "event_update_id", "severity", "final_score_snapshot", "threshold_snapshot", "heat_score_snapshot", "momentum_score_snapshot", "breadth_score_snapshot", "reason_codes", "fingerprint", "triggered_at", "created_at"},
		"alert_email_deliveries":       {"id", "occurrence_id", "idempotency_key", "severity", "status", "next_attempt_at", "succeeded_at"},
		"alert_email_attempts":         {"id", "delivery_id", "attempt_no", "status"},
		"alert_state_audits":           {"id", "alert_thread_id", "actor_type", "actor_user_id", "from_state", "to_state", "expected_version", "reason_code", "created_at"},
		"notification_events":          {"id", "event_type", "resource_type", "resource_id", "audience_role", "occurred_at", "payload", "dedupe_key", "created_at"},
		"source_credentials":           {"id", "source_connection_id", "key_version", "nonce", "ciphertext", "updated_at"},
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
	if got, want := len(seen), 67; got != want {
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
