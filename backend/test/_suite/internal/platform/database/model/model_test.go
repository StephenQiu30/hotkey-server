package model

import "testing"

func TestSpecsHaveUniqueTablesAndColumns(t *testing.T) {
	seen := map[string]bool{}
	wantColumns := map[string][]string{
		"monitor_intent_drafts":                 {"id", "resource_version", "monitor_id", "config_version_id", "created_at", "updated_at"},
		"monitor_intent_draft_revisions":        {"id", "version", "draft_id", "monitor_id", "config_version_id", "resource_version", "objective", "created_at"},
		"monitor_intent_clauses":                {"id", "version", "revision_id", "draft_id", "resource_version", "ordinal", "operator", "field", "value"},
		"monitor_intent_entities":               {"id", "version", "revision_id", "draft_id", "resource_version", "ordinal", "canonical_id", "display_name", "ambiguity_note"},
		"monitor_intent_entity_aliases":         {"id", "version", "entity_id", "draft_id", "resource_version", "ordinal", "alias"},
		"monitor_intent_examples":               {"id", "version", "revision_id", "draft_id", "resource_version", "ordinal", "label", "example_text"},
		"monitor_intent_analysis_runs":          {"id", "monitor_id", "draft_id", "draft_resource_version", "kind", "input_hash", "profile_version", "sample_limit", "request_hash", "idempotency_key", "river_job_id", "status", "queued_at", "started_at", "completed_at", "invalidated_at", "failure_reason", "result_fingerprint"},
		"monitor_intent_expansion_candidates":   {"id", "version", "draft_id", "introduced_resource_version", "candidate_id", "origin_run_id", "candidate_value", "source", "reason", "model_version", "prompt_version", "input_hash", "similarity", "risk"},
		"monitor_intent_draft_candidates":       {"id", "version", "revision_id", "draft_id", "resource_version", "candidate_record_id", "ordinal", "approval_status", "reviewer_user_id", "reviewed_at", "review_note"},
		"monitor_intent_mutation_receipts":      {"id", "monitor_id", "draft_id", "mutation_kind", "idempotency_key", "command_fingerprint", "expected_resource_version", "result_resource_version", "created_at"},
		"monitor_intent_preview_results":        {"run_id", "estimated_alert_count", "created_at"},
		"monitor_intent_preview_samples":        {"id", "run_id", "ordinal", "document_version_id", "title", "decision"},
		"monitor_intent_preview_recall_signals": {"id", "sample_id", "run_id", "ordinal", "channel", "rank", "score"},
		"monitor_intent_preview_reasons":        {"id", "sample_id", "run_id", "ordinal", "reason_type", "reason"},
		"monitor_intent_preview_warnings":       {"id", "run_id", "ordinal", "warning"},
		"monitor_compiled_profiles":             {"id", "version", "monitor_id", "purpose", "config_version_id", "monitor_version_id", "source_preview_compiled_profile_id", "preview_run_id", "draft_id", "draft_resource_version", "intent_revision_id", "compiler_version", "matching_algorithm_version", "lexical_algorithm_version", "semantic_algorithm_version", "structured_algorithm_version", "search_normalization_profile_version", "semantic_state", "semantic_unavailable_reason", "status", "profile_hash", "ready_at", "retired_at", "created_at"},
		"monitor_compiled_clauses":              {"id", "version", "compiled_profile_id", "ordinal", "operator", "field", "value", "normalized_value", "origin", "created_at"},
		"monitor_compiled_entities":             {"id", "version", "compiled_profile_id", "ordinal", "canonical_id", "created_at"},
		"monitor_compiled_entity_aliases":       {"id", "version", "compiled_entity_id", "compiled_profile_id", "ordinal", "alias", "normalized_alias", "created_at"},
		"monitor_compiled_intent_embeddings":    {"id", "compiled_profile_id", "config_version_id", "model_profile_id", "model_profile_version", "model_version", "input_hash", "embedding", "ai_run_id", "created_at"},
		"source_rights_policies":                {"id", "version", "recorded_by_user_id", "idempotency_key", "command_fingerprint", "source_connection_id", "scope_type", "scope_subject", "policy_revision", "policy_hash"},
		"source_rights_decision_batches":        {"id", "version", "source_connection_id", "policy_id", "expected_policy_version", "subject_type", "subject_key", "input_digest", "recorded_by_user_id", "idempotency_key", "command_fingerprint", "decision_count"},
		"source_rights_decisions":               {"id", "decision_batch_id", "source_connection_id", "policy_id", "policy_revision", "policy_scope_type", "policy_scope_subject", "priority_rank", "basis_summary", "subject_type", "subject_key", "input_digest", "action", "decision", "effective_from", "retention_days", "supersedes_decision_id"},
		"evidence_snapshots":                    {"id", "source_connection_id", "store_raw_rights_decision_id", "retain_rights_decision_id", "snapshot_key", "object_key", "payload_sha256", "collector_profile_version", "retention_until", "lifecycle_state"},
		"source_observations":                   {"id", "version", "source_connection_id", "collection_run_item_id", "external_id", "upstream_identity", "body_origin", "completeness"},
		"source_observation_evidences":          {"id", "source_connection_id", "source_observation_id", "evidence_snapshot_id", "usage", "locator_type", "locator_value", "selected_payload_sha256"},
		"documents":                             {"id", "version", "source_connection_id", "document_key", "current_document_version_id", "document_state"},
		"document_versions":                     {"id", "version", "document_id", "source_observation_id", "revision_no", "version_key", "quality_score", "content_sha256", "extractor_profile_version", "extractor_profile_sha256", "display_private_rights_decision_id", "lifecycle_state"},
		"document_identity_keys":                {"id", "version", "source_connection_id", "document_id", "identity_kind", "identity_value"},
		"derived_artifacts":                     {"id", "source_connection_id", "document_version_id", "store_derived_rights_decision_id", "retain_rights_decision_id", "artifact_type", "transformer_profile_sha256", "vault_relative_path", "sha256", "anchor_normalization_version", "anchor_map_profile_version", "anchor_plaintext_sha256", "anchor_markdown_sha256", "anchor_map_sha256", "retention_until", "lifecycle_state", "active"},
		"document_anchor_blocks":                {"id", "derived_artifact_id", "anchor_map_sha256", "block_ordinal", "plaintext_utf8_byte_start", "plaintext_utf8_byte_end", "markdown_utf8_byte_start", "markdown_utf8_byte_end", "markdown_anchor", "created_at"},
		"document_text_quote_selectors":         {"id", "version", "source_connection_id", "document_version_id", "plaintext_artifact_id", "markdown_artifact_id", "quote_rights_decision_id", "retain_rights_decision_id", "exact_quote", "prefix", "suffix", "utf8_byte_start", "utf8_byte_end", "quote_sha256", "plaintext_sha256", "normalization_version", "selector_version", "anchor_map_sha256", "markdown_anchor", "retention_until", "created_at"},
		"document_version_search_indexes":       {"id", "version", "document_version_id", "source_connection_id", "derived_artifact_id", "store_derived_rights_decision_id", "retain_rights_decision_id", "normalization_profile_version", "normalized_text_sha256", "title_search_vector", "body_search_vector", "title_trigrams", "body_trigrams", "entity_keys", "action_keys", "location_keys", "region_keys", "lifecycle_state", "tombstoned_at", "purge_reason", "retention_until", "indexed_at", "created_at"},
		"document_version_embeddings":           {"id", "document_version_id", "source_connection_id", "embed_local_rights_decision_id", "retain_rights_decision_id", "model_profile_id", "model_profile_version", "model_version", "normalized_text_sha256", "embedding", "ai_run_id", "retention_until", "lifecycle_state", "tombstoned_at", "purge_reason", "created_at"},
		"relevance_evaluation_runs":             {"id", "version", "dataset_version", "dataset_hash", "family_isolation_hash", "annotation_protocol_version", "annotation_guideline_sha256", "split_strategy_version", "annotator_count", "agreement_metric", "agreement_score", "time_boundary", "sample_window_start", "sample_window_end", "matching_algorithm_version", "reranker_version", "calibration_version", "calibration_slope", "calibration_intercept", "reject_threshold", "accept_threshold", "sample_count", "positive_count", "negative_count", "recall_at_100", "precision_score", "recall_score", "expected_calibration_error", "brier_score", "precision_wilson_lower", "hard_negative_count", "hard_negative_passed", "status", "evaluated_by_user_id", "evaluated_at", "created_at"},
		"relevance_evaluation_slices":           {"id", "version", "evaluation_run_id", "dimension", "value", "sample_count", "positive_count", "negative_count", "precision_score", "recall_score", "passed", "created_at"},
		"relevance_decision_profiles":           {"id", "version", "profile_name", "matching_algorithm_version", "reranker_version", "calibration_version", "status", "reject_threshold", "accept_threshold", "calibration_slope", "calibration_intercept", "evaluation_sample_count", "evaluation_run_id", "created_by_user_id", "activated_by_user_id", "activated_at", "rolled_back_by_user_id", "rolled_back_at", "created_at"},
		"decision_quality_evaluation_runs":      {"id", "version", "module", "profile_version", "dataset_version", "dataset_sha256", "annotation_protocol_version", "annotation_guideline_sha256", "split_strategy_version", "family_isolation_sha256", "event_isolation_sha256", "annotator_count", "agreement_metric", "agreement_score", "time_boundary", "sample_count", "positive_count", "negative_count", "precision_score", "recall_score", "precision_wilson_lower", "false_merge_rate", "pairwise_precision", "b_cubed_f1", "ceaf_e", "cluster_count_ratio", "locator_accuracy", "provenance_completeness", "evidence_relation_macro_f1", "hotspot_precision", "median_discovery_delay_seconds", "passed", "reason_codes", "evaluated_by_user_id", "evaluated_at", "created_at"},
		"decision_quality_evaluation_slices":    {"id", "version", "evaluation_run_id", "module", "dimension", "value", "sample_count", "precision_score", "recall_score", "passed", "created_at"},
		"decision_quality_profiles":             {"id", "version", "module", "profile_version", "status", "evaluation_run_id", "created_by_user_id", "activated_by_user_id", "activated_at", "rolled_back_by_user_id", "rolled_back_at", "created_at", "updated_at"},
		"document_match_decisions":              {"id", "version", "monitor_id", "monitor_version_id", "compiled_profile_id", "document_version_id", "relevance_profile_id", "matching_algorithm_version", "reranker_version", "calibration_version", "rrf_score", "relevance_probability", "decision", "degraded", "reason_codes", "input_hash", "decided_at"},
		"document_match_recall_signals":         {"id", "version", "match_decision_id", "ordinal", "channel", "rank", "raw_score", "algorithm_version", "created_at"},
		"document_match_overrides":              {"id", "version", "match_decision_id", "sequence_no", "monitor_id", "monitor_version_id", "document_version_id", "previous_effective_decision", "decision", "reason_code", "note", "actor_user_id", "idempotency_key", "command_fingerprint", "created_at"},
		"ai_model_profiles":                     {"id", "version", "name", "task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions", "timeout_seconds", "max_attempts", "max_cost", "daily_budget", "fallback_priority", "enabled", "deleted_at"},
		"ai_runs":                               {"id", "owning_job_id", "workspace_key", "skill_id", "task_type", "target_type", "target_id", "target_version", "runtime_version", "model_profile_id", "model_profile_version", "model_version", "prompt_version", "input_schema_version", "schema_version", "parameters_version", "input_hash", "evidence_set_hash", "reuse_key", "attempt", "max_attempts", "repair_attempted", "retry_after", "error_code", "budget_day", "reserved_cost", "lease_expires_at", "status"},
		"ai_budget_ledgers":                     {"id", "model_profile_id", "budget_day", "reserved_cost", "settled_cost", "overage_blocked", "updated_at"},
		"quota_usage_ledgers":                   {"id", "dimension", "subject_type", "subject_id", "window_start", "window_end", "used", "updated_at"},
		"source_request_usage_ledgers":          {"id", "source_connection_id", "resource_profile_version", "budget_day", "used", "updated_at"},
		"content_embeddings":                    {"id", "content_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"},
		"monitor_embeddings":                    {"id", "monitor_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "query_text", "embedding", "active"},
		"event_embeddings":                      {"id", "event_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"},
		"topic_embeddings":                      {"id", "topic_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"},
		"auth_sessions":                         {"id", "user_id", "family_id", "absolute_expires_at", "revoked_at"},
		"auth_refresh_tokens":                   {"id", "session_id", "token_hash", "expires_at", "used_at", "revoked_at"},
		"monitors":                              {"id", "version", "name", "status", "draft_config_version_id", "published_config_version_id", "deleted_at"},
		"monitor_config_versions":               {"id", "version", "monitor_id", "revision", "state", "config_hash", "published_at"},
		"monitor_rules":                         {"id", "version", "config_version_id", "rule_type", "value"},
		"monitor_sources":                       {"id", "version", "config_version_id", "source_connection_id", "query_signature"},
		"source_checkpoints":                    {"id", "monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"},
		"collection_runs":                       {"id", "source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "status", "updated_at"},
		"collection_run_targets":                {"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status", "updated_at"},
		"contents":                              {"id", "source_connection_id", "external_id", "dedupe_key", "dedupe_reason", "dedupe_version", "view_count", "like_count", "comment_count", "share_count", "deleted_at"},
		"monitor_matches":                       {"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "input_hash", "scoring_version", "final_score", "decision", "decision_origin", "embedding_model_profile_id", "embedding_model_profile_version", "embedding_model_version", "review_ai_run_id"},
		"monitor_match_feedbacks":               {"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "monitor_match_id", "actor_user_id", "feedback_type"},
		"monitor_feedback_suggestions":          {"id", "version", "monitor_id", "monitor_config_version_id", "suggestion_type", "value", "support_count", "status", "reviewed_by_user_id"},
		"collection_run_items":                  {"id", "run_id", "source_connection_id", "source_code", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "outcome", "observed_at"},
		"collection_run_target_items":           {"id", "collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"},
		"content_metric_snapshots":              {"id", "content_id", "captured_at", "view_count", "like_count", "comment_count", "share_count"},
		"metric_capability_profiles":            {"id", "version", "source_type", "profile_version", "status", "published_at", "archived_at"},
		"event_updates":                         {"id", "version", "event_id", "sequence_no", "kind", "summary", "observed_at", "reason_codes", "before_state", "after_state", "evidence_set_hash", "idempotency_key", "created_at"},
		"alert_threads":                         {"id", "version", "monitor_id", "monitor_config_version_id", "monitor_revision", "monitor_config_hash", "event_id", "trigger_type", "policy_version", "state", "severity", "event_threshold_snapshot", "alert_min_heat_snapshot", "alert_min_momentum_snapshot", "alert_min_breadth_snapshot", "alert_warning_threshold_snapshot", "alert_critical_threshold_snapshot", "alert_cooldown_minutes_snapshot", "title_snapshot", "reason_snapshot", "first_triggered_at", "last_triggered_at", "occurrence_count", "cooldown_until", "acknowledged_at", "acknowledged_by_user_id", "resolved_at", "resolved_by_user_id", "suppressed_at", "suppressed_by_user_id", "created_at", "updated_at"},
		"alert_occurrences":                     {"id", "alert_thread_id", "event_update_id", "severity", "final_score_snapshot", "threshold_snapshot", "heat_score_snapshot", "momentum_score_snapshot", "breadth_score_snapshot", "reason_codes", "fingerprint", "triggered_at", "created_at"},
		"alert_email_deliveries":                {"id", "occurrence_id", "idempotency_key", "severity", "status", "next_attempt_at", "succeeded_at"},
		"alert_email_attempts":                  {"id", "delivery_id", "attempt_no", "status"},
		"alert_state_audits":                    {"id", "alert_thread_id", "actor_type", "actor_user_id", "from_state", "to_state", "expected_version", "reason_code", "created_at"},
		"notification_events":                   {"id", "event_type", "resource_type", "resource_id", "audience_role", "occurred_at", "payload", "dedupe_key", "created_at"},
		"notification_outbox_events":            {"id", "version", "event_type", "resource_type", "resource_id", "resource_version", "monitor_id", "occurred_at", "title", "summary", "resource_status", "deep_link", "dedupe_key", "created_at"},
		"user_notifications":                    {"id", "version", "outbox_event_id", "user_id", "monitor_id", "event_type", "resource_type", "resource_id", "resource_version", "occurred_at", "title", "summary", "resource_status", "deep_link", "created_at"},
		"notification_delivery_attempts":        {"id", "version", "user_notification_id", "channel", "delivery_target_key", "attempt_no", "status", "provider_message_id", "response_code", "error_code", "attempted_at", "created_at"},
		"notification_delivery_claims":          {"user_notification_id", "channel", "delivery_target_key", "claim_token", "claimed_at", "lease_until"},
		"web_push_subscriptions":                {"id", "version", "user_id", "endpoint_sha256", "endpoint_ciphertext", "p256dh_ciphertext", "auth_ciphertext", "encryption_key_version", "device_label", "timezone", "quiet_start", "quiet_end", "ttl_seconds", "status", "expiration_reason", "last_success_at", "last_failure_at", "idempotency_key", "command_fingerprint", "created_at", "updated_at"},
		"web_push_subscription_monitors":        {"id", "subscription_id", "monitor_id", "created_at"},
		"source_credentials":                    {"id", "source_connection_id", "key_version", "nonce", "ciphertext", "updated_at"},
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
	if got, want := len(seen), 152; got != want {
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
		wantedVersion := "version"
		if spec.Table == "monitor_intent_drafts" {
			wantedVersion = "resource_version"
		}
		if spec.Lifecycle == LifecycleBusiness && metadata.VersionColumn != wantedVersion {
			t.Errorf("business table %s VersionColumn = %q, want %s", spec.Table, metadata.VersionColumn, wantedVersion)
		}
		if spec.Lifecycle == LifecycleOperational && metadata.VersionColumn != "" {
			t.Errorf("operational table %s VersionColumn = %q, want empty", spec.Table, metadata.VersionColumn)
		}
	}
}

func TestMonitorIntentPersistenceMetadataKeepsDraftResourceVersionSeparate(t *testing.T) {
	metadata, found := PersistenceFor("monitor_intent_drafts")
	if !found || metadata.VersionColumn != "resource_version" || metadata.Deletion != DeletionRetained {
		t.Fatalf("monitor intent draft persistence = %#v found=%t", metadata, found)
	}
}

func TestMonitorIntentPreviewPersistenceUsesItsRunIdentity(t *testing.T) {
	metadata, found := PersistenceFor("monitor_intent_preview_results")
	if !found || metadata.KeyColumn != "run_id" || !sameColumns(metadata.CursorFields, []string{"run_id"}) {
		t.Fatalf("monitor intent preview persistence = %#v found=%t", metadata, found)
	}
}

func TestImmutableEvidenceAndTombstonedBusinessFactsAreRetained(t *testing.T) {
	for _, table := range []string{"source_rights_policies", "source_observations", "documents", "document_versions"} {
		metadata, found := PersistenceFor(table)
		if !found {
			t.Fatalf("PersistenceFor(%q) did not return metadata", table)
		}
		if metadata.Deletion != DeletionRetained {
			t.Errorf("%s deletion = %q, want retained", table, metadata.Deletion)
		}
	}
}
