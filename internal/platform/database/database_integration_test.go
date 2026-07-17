package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestEmbeddedSchemaCatalogIsComplete(t *testing.T) {
	tables := EmbeddedSchemaTableNames()
	if got, want := len(tables), 62; got != want {
		t.Fatalf("embedded table count = %d, want %d", got, want)
	}
	if !EmbeddedSchemaContains("CREATE EXTENSION IF NOT EXISTS vector") {
		t.Fatal("embedded schema does not require pgvector")
	}
}

func TestRuntimeUsesSharedPoolAndVerifiesCatalog(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	verification, err := Verify(context.Background(), runtime.Pool)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got, want := len(verification.Tables), 62; got != want {
		t.Fatalf("verified table count = %d, want %d", got, want)
	}
	if verification.CatalogFingerprint == "" {
		t.Fatal("catalog fingerprint is empty")
	}
	if err := runtime.GORM.WithContext(context.Background()).Exec("SELECT 1").Error; err != nil {
		t.Fatalf("GORM facade query: %v", err)
	}
	if err := runtime.Ping(context.Background()); err != nil {
		t.Fatalf("Pool ping: %v", err)
	}
}

func TestAIBaseSchemaEnforcesProfileRunAndLedgerContracts(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	profileSQL := `
INSERT INTO ai_model_profiles (
    name, task_type, provider, model_name, credential_ref, model_version,
    embedding_dimensions, max_attempts, max_cost, daily_budget
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	for _, test := range []struct {
		name       string
		taskType   string
		provider   string
		credential any
		dimension  any
		maxCost    string
		daily      any
	}{
		{"future task", "classification", "openai", "env:OPENAI_API_KEY", 1024, "1.0000", "10.0000"},
		{"onnx term expansion", "term_expansion", "onnx", nil, nil, "1.0000", "10.0000"},
		{"non-positive max cost", "embedding", "openai", "env:OPENAI_API_KEY", 1024, "0.0000", "10.0000"},
		{"daily budget below max", "embedding", "openai", "env:OPENAI_API_KEY", 1024, "2.0000", "1.0000"},
	} {
		if _, err := runtime.SQL.Exec(profileSQL,
			"plan008-invalid-"+test.name+"-"+suffix, test.taskType, test.provider,
			"fixture-model", test.credential, "profile-v1", test.dimension, 1, test.maxCost, test.daily,
		); err == nil {
			t.Errorf("insert profile with %s error = nil, want CHECK rejection", test.name)
		} else {
			assertPostgreSQLState(t, err, "23514")
		}
	}

	var profileID int64
	if err := runtime.SQL.QueryRow(profileSQL+" RETURNING id",
		"plan008-valid-"+suffix, "embedding", "openai", "fixture-model", "env:OPENAI_API_KEY", "profile-v1", 1024, 2, "1.0000", "10.0000",
	).Scan(&profileID); err != nil {
		t.Fatalf("insert valid AI profile: %v", err)
	}

	if _, err := runtime.SQL.Exec(`
INSERT INTO ai_budget_ledgers (model_profile_id, budget_day, reserved_cost, settled_cost)
VALUES ($1, current_date, 0, 0)`, profileID); err != nil {
		t.Fatalf("insert AI budget ledger: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO ai_budget_ledgers (model_profile_id, budget_day, reserved_cost, settled_cost)
VALUES ($1, current_date, -1, 0)`, profileID); err == nil {
		t.Fatal("negative AI budget reservation error = nil, want CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO ai_budget_ledgers (model_profile_id, budget_day, reserved_cost, settled_cost)
VALUES ($1, current_date, 0, 0)`, profileID); err == nil {
		t.Fatal("duplicate AI budget ledger error = nil, want unique rejection")
	} else {
		assertPostgreSQLState(t, err, "23505")
	}

	const runSQL = `
INSERT INTO ai_runs (
    task_type, target_type, target_id, model_profile_id, prompt_version, schema_version,
    input_hash, status, model_profile_version, model_version, parameters_version,
    input_schema_version, evidence_set_hash, reuse_key, attempt, max_attempts,
    budget_day, reserved_cost, lease_expires_at
) VALUES (
    'embedding', 'content', 1, $1, 'prompt-v1', 'output-v1', $2, $3, 1,
    'profile-v1', 'parameters-v1', 'input-v1', $4, $5, 1, 2, current_date, 0, $6
)`
	inputHash := strings.Repeat("a", 64)
	evidenceHash := strings.Repeat("b", 64)
	successReuseKey := strings.Repeat("c", 64)
	if _, err := runtime.SQL.Exec(runSQL, profileID, inputHash, "succeeded", evidenceHash, successReuseKey, nil); err != nil {
		t.Fatalf("insert succeeded AI run: %v", err)
	}
	if _, err := runtime.SQL.Exec(runSQL, profileID, inputHash, "succeeded", evidenceHash, successReuseKey, nil); err == nil {
		t.Fatal("duplicate succeeded AI run error = nil, want unique rejection")
	} else {
		assertPostgreSQLState(t, err, "23505")
	}
	inflightReuseKey := strings.Repeat("d", 64)
	if _, err := runtime.SQL.Exec(runSQL, profileID, inputHash, "queued", evidenceHash, inflightReuseKey, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("insert queued AI run: %v", err)
	}
	if _, err := runtime.SQL.Exec(runSQL, profileID, inputHash, "queued", evidenceHash, inflightReuseKey, time.Now().UTC().Add(time.Minute)); err == nil {
		t.Fatal("duplicate in-flight AI run error = nil, want unique rejection")
	} else {
		assertPostgreSQLState(t, err, "23505")
	}
	if _, err := runtime.SQL.Exec(runSQL, profileID, inputHash, "queued", evidenceHash, strings.Repeat("e", 64), nil); err == nil {
		t.Fatal("queued AI run without lease error = nil, want CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}

	for _, indexName := range []string{
		"ai_runs_reuse_succeeded_uq",
		"ai_runs_reuse_inflight_uq",
		"content_embeddings_one_active_per_profile_uq",
		"monitor_embeddings_one_active_per_profile_uq",
		"event_embeddings_one_active_per_profile_uq",
		"topic_embeddings_one_active_per_profile_uq",
	} {
		var exists bool
		if err := runtime.SQL.QueryRow("SELECT to_regclass($1) IS NOT NULL", "public."+indexName).Scan(&exists); err != nil {
			t.Fatalf("check AI index %s: %v", indexName, err)
		}
		if !exists {
			t.Errorf("required AI index %s does not exist", indexName)
		}
	}
}

func TestSessionSchemaEnforcesIdentityConstraints(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	now := time.Now().UTC().Truncate(time.Microsecond)
	var userID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email, password_hash, display_name, role)
VALUES ($1, 'hash', 'identity schema', 'viewer')
RETURNING id`, fmt.Sprintf("identity-schema-%d@example.test", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatalf("create identity schema user: %v", err)
	}

	sessionExpiry := now.Add(30 * 24 * time.Hour)
	const familyID = "12345678-1234-1234-1234-123456789abc"
	var sessionID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at)
VALUES ($1, $2, $3)
RETURNING id`, userID, familyID, sessionExpiry).Scan(&sessionID); err != nil {
		t.Fatalf("create logical auth session: %v", err)
	}

	_, err := runtime.SQL.Exec(`
INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at)
VALUES ($1, $2, $3)`, userID, familyID, sessionExpiry)
	assertPostgreSQLState(t, err, "23505")

	_, err = runtime.SQL.Exec(`
INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at, created_at)
VALUES ($1, '87654321-4321-4321-4321-cba987654321', $2, $3)`, userID, now, now.Add(time.Second))
	assertPostgreSQLState(t, err, "23514")

	const tokenHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := runtime.SQL.Exec(`
INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
VALUES ($1, $2, $3)`, sessionID, tokenHash, now.Add(7*24*time.Hour)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	_, err = runtime.SQL.Exec(`
UPDATE auth_sessions
SET absolute_expires_at = $1
WHERE id = $2`, sessionExpiry.Add(time.Hour), sessionID)
	assertPostgreSQLState(t, err, "23514")

	var actualSessionExpiry, actualTokenExpiry time.Time
	if err := runtime.SQL.QueryRow(`
SELECT session.absolute_expires_at, token.expires_at
FROM auth_sessions AS session
JOIN auth_refresh_tokens AS token ON token.session_id = session.id
WHERE session.id = $1 AND token.token_hash = $2`, sessionID, tokenHash).Scan(&actualSessionExpiry, &actualTokenExpiry); err != nil {
		t.Fatalf("read session and refresh token after rejected expiry update: %v", err)
	}
	if !actualSessionExpiry.Equal(sessionExpiry) {
		t.Errorf("session absolute expiry after rejected update = %s, want %s", actualSessionExpiry, sessionExpiry)
	}
	if want := now.Add(7 * 24 * time.Hour); !actualTokenExpiry.Equal(want) {
		t.Errorf("refresh token expiry after rejected session update = %s, want %s", actualTokenExpiry, want)
	}

	_, err = runtime.SQL.Exec(`
INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
VALUES ($1, $2, $3)`, sessionID, tokenHash, now.Add(6*24*time.Hour))
	assertPostgreSQLState(t, err, "23505")

	_, err = runtime.SQL.Exec(`
INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
VALUES ($1, $2, $3)`, sessionID+1_000_000, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", now.Add(7*24*time.Hour))
	assertPostgreSQLState(t, err, "23503")

	_, err = runtime.SQL.Exec(`
INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
VALUES ($1, $2, $3)`, sessionID, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", sessionExpiry.Add(time.Second))
	assertPostgreSQLState(t, err, "23514")

	for _, indexName := range []string{
		"auth_sessions_active_user_idx",
		"auth_refresh_tokens_session_expiry_idx",
	} {
		var exists bool
		if err := runtime.SQL.QueryRow("SELECT to_regclass($1) IS NOT NULL", "public."+indexName).Scan(&exists); err != nil {
			t.Fatalf("check index %s: %v", indexName, err)
		}
		if !exists {
			t.Errorf("required auth access-path index %s does not exist", indexName)
		}
	}
}

func TestMonitorConfigurationSchemaEnforcesHistoricalConstraints(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())
	var sourceID, firstMonitorID, secondMonitorID, firstConfigID, secondConfigID, firstMonitorSourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, 'https://example.test/feed')
RETURNING id`, "monitor-schema-source-"+suffix).Scan(&sourceID); err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO source_connections (source_type, name, endpoint, config)
VALUES ('rss', $1, 'https://example.test/invalid', '{"secret":"forbidden"}'::jsonb)`, "monitor-schema-invalid-source-"+suffix); err == nil {
		t.Fatal("unknown source config key = nil error, want whitelist rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "monitor-schema-first-"+suffix).Scan(&firstMonitorID); err != nil {
		t.Fatalf("create first monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "monitor-schema-second-"+suffix).Scan(&secondMonitorID); err != nil {
		t.Fatalf("create second monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_config_versions (monitor_id, revision)
VALUES ($1, 1)
RETURNING id`, firstMonitorID).Scan(&firstConfigID); err != nil {
		t.Fatalf("create first draft configuration: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, weight, approval_status)
VALUES ($1, 'keyword', 'contains', 'schema history', 10, 'approved')`, firstConfigID); err != nil {
		t.Fatalf("create draft rule: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_sources (config_version_id, source_connection_id)
VALUES ($1, $2)
RETURNING id`, firstConfigID, sourceID).Scan(&firstMonitorSourceID); err != nil {
		t.Fatalf("create draft monitor source: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_config_versions
SET state = 'published', config_hash = $1, published_at = $2
WHERE id = $3`, strings.Repeat("a", 64), now, firstConfigID); err != nil {
		t.Fatalf("publish configuration: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET published_config_version_id = $1 WHERE id = $2`, firstConfigID, firstMonitorID); err != nil {
		t.Fatalf("set published configuration pointer: %v", err)
	}
	for _, constraint := range []string{"monitors_draft_config_version_fkey", "monitors_published_config_version_fkey"} {
		assertMonitorPointerForeignKey(t, runtime, constraint)
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM monitor_config_versions WHERE id = $1`, firstConfigID); err == nil {
		t.Fatal("delete monitor configuration referenced by a pointer = nil error, want ON DELETE RESTRICT")
	} else {
		assertPostgreSQLRestrictViolation(t, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET timezone = 'Asia/Shanghai' WHERE id = $1`, firstConfigID); err == nil {
		t.Fatal("published configuration update = nil error, want immutable trigger rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_rules SET value = 'changed' WHERE config_version_id = $1`, firstConfigID); err == nil {
		t.Fatal("published rule update = nil error, want immutable trigger rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_sources SET priority = 1 WHERE id = $1`, firstMonitorSourceID); err == nil {
		t.Fatal("published monitor source update = nil error, want immutable trigger rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`UPDATE source_connections SET endpoint = 'https://example.test/changed' WHERE id = $1`, sourceID); err == nil {
		t.Fatal("published source semantic update = nil error, want immutable trigger rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_config_versions (monitor_id, revision)
VALUES ($1, 1)
RETURNING id`, secondMonitorID).Scan(&secondConfigID); err != nil {
		t.Fatalf("create second draft configuration: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET published_config_version_id = $1 WHERE id = $2`, firstConfigID, secondMonitorID); err == nil {
		t.Fatal("wrong monitor configuration pointer = nil error, want owner rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}

	var contentID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (source_connection_id, external_id, content_type, canonical_url, published_at, fetched_at, dedupe_key)
VALUES ($1, $2, 'article', 'https://example.test/article', $3, $3, $4)
RETURNING id`, sourceID, "monitor-schema-content-"+suffix, now, strings.Repeat("b", 64)).Scan(&contentID); err != nil {
		t.Fatalf("create content for monitor history: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_matches (
  monitor_id, monitor_config_version_id, content_id, rule_score, final_score, decision, algorithm_version,
  input_hash, scoring_version
)
VALUES ($1, $2, $3, 10, 10, 'accepted', 'schema-test', $4, 'schema-v1')`, secondMonitorID, firstConfigID, contentID, strings.Repeat("c", 64)); err == nil {
		t.Fatal("wrong monitor/config historical match = nil error, want composite foreign key rejection")
	} else {
		assertPostgreSQLState(t, err, "23503")
	}

	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at)
VALUES ($1, $2, $3, $4, 'manual', $3)
RETURNING id`, sourceID, strings.Repeat("c", 64), now, now.Add(time.Minute)).Scan(&runID); err != nil {
		t.Fatalf("create shared collection run: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_targets (collection_run_id, monitor_source_id, monitor_config_version_id)
VALUES ($1, $2, $3)`, runID, firstMonitorSourceID, secondConfigID); err == nil {
		t.Fatal("wrong monitor source/config run target = nil error, want composite foreign key rejection")
	} else {
		assertPostgreSQLState(t, err, "23503")
	}
}

func TestCollectionCaptureSchemaEnforcesDurableReconciliation(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())
	var sourceID, monitorID, configID, monitorSourceID, runID, targetID, itemID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, 'https://example.test/capture')
RETURNING id`, "collection-capture-source-"+suffix).Scan(&sourceID); err != nil {
		t.Fatalf("create collection source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "collection-capture-monitor-"+suffix).Scan(&monitorID); err != nil {
		t.Fatalf("create collection monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_config_versions (monitor_id, revision)
VALUES ($1, 1)
RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create collection monitor config: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_sources (config_version_id, source_connection_id)
VALUES ($1, $2)
RETURNING id`, configID, sourceID).Scan(&monitorSourceID); err != nil {
		t.Fatalf("create collection monitor source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (
    source_connection_id, query_signature, request_cursor, next_cursor, etag, last_modified,
    retry_after, page_count, window_start, window_end, trigger_type, scheduled_at
)
VALUES ($1, $2, 'before', 'after', 'etag', 'last-modified', $3, 1, $3, $4, 'manual', $3)
RETURNING id`, sourceID, strings.Repeat("d", 64), now, now.Add(time.Minute)).Scan(&runID); err != nil {
		t.Fatalf("create detailed collection run: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_run_targets (collection_run_id, monitor_source_id, monitor_config_version_id)
VALUES ($1, $2, $3)
RETURNING id`, runID, monitorSourceID, configID).Scan(&targetID); err != nil {
		t.Fatalf("create collection target: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, 'rss', 'item-1', 'article', 'v1', '{"title":"safe"}'::jsonb, $3, 'discarded', 'captured', $4)
RETURNING id`, runID, sourceID, strings.Repeat("e", 64), now).Scan(&itemID); err != nil {
		t.Fatalf("write durable captured collection item: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_target_items (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
VALUES ($1, $2, $3, 'captured')`, runID, targetID, itemID); err != nil {
		t.Fatalf("write target-item reconciliation: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO source_checkpoints (monitor_source_id, query_hash, last_successful_run_id, last_fetched_at, next_poll_at)
VALUES ($1, $2, $3, $4, $5)`, monitorSourceID, strings.Repeat("f", 64), runID, now, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("write successful capture checkpoint: %v", err)
	}
	var otherRunID, otherTargetID, otherItemID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at)
VALUES ($1, $2, $3, $4, 'manual', $3)
RETURNING id`, sourceID, strings.Repeat("b", 64), now, now.Add(time.Minute)).Scan(&otherRunID); err != nil {
		t.Fatalf("create second collection run: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_run_targets (collection_run_id, monitor_source_id, monitor_config_version_id)
VALUES ($1, $2, $3)
RETURNING id`, otherRunID, monitorSourceID, configID).Scan(&otherTargetID); err != nil {
		t.Fatalf("create second collection target: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, 'rss', 'item-2', 'article', 'v1', '{"title":"other"}'::jsonb, $3, 'discarded', 'captured', $4)
RETURNING id`, otherRunID, sourceID, strings.Repeat("c", 64), now).Scan(&otherItemID); err != nil {
		t.Fatalf("write second collection item: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_target_items (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
VALUES ($1, $2, $3, 'captured')`, runID, targetID, otherItemID); err == nil {
		t.Fatal("cross-run target-item reconciliation = nil error, want run-alignment foreign key rejection")
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_target_items (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
VALUES ($1, $2, $3, 'captured')`, otherRunID, otherTargetID, itemID); err == nil {
		t.Fatal("reverse cross-run target-item reconciliation = nil error, want run-alignment foreign key rejection")
	}

	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, 'rss', 'item-1', 'article', 'v1', '{"title":"safe"}'::jsonb, $3, 'discarded', 'captured', $4)`, runID, sourceID, strings.Repeat("e", 64), now); err == nil {
		t.Fatal("duplicate run item = nil error, want unique reconciliation key rejection")
	} else {
		assertPostgreSQLState(t, err, "23505")
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_target_items (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
VALUES ($1, $2, $3, 'captured')`, runID, targetID, itemID); err == nil {
		t.Fatal("duplicate target-item reconciliation = nil error, want unique rejection")
	} else {
		assertPostgreSQLState(t, err, "23505")
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_items (
    run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, $2, 'rss', 'item-invalid', 'article', 'v1', '{"title":"safe"}'::jsonb, $3, 'raw_response', 'captured', $4)`, runID, sourceID, strings.Repeat("a", 64), now); err == nil {
		t.Fatal("unapproved raw payload disposition = nil error, want CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
}

func TestPlan007SchemaUpgradeBackfillsCaptureSourceAndPreservesMetricPolicy(t *testing.T) {
	runtime := openEmptyTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	if _, err := runtime.SQL.Exec(`
CREATE TABLE source_connections (id bigint PRIMARY KEY);
CREATE TABLE contents (
    id bigint PRIMARY KEY,
    source_connection_id bigint NOT NULL REFERENCES source_connections(id),
    content_status varchar(16) NOT NULL,
    duplicate_of_id bigint,
    view_count bigint NOT NULL DEFAULT 0,
    like_count bigint NOT NULL DEFAULT 0,
    comment_count bigint NOT NULL DEFAULT 0,
    share_count bigint NOT NULL DEFAULT 0
);
CREATE TABLE content_metric_snapshots (
    id bigint PRIMARY KEY,
    content_id bigint NOT NULL REFERENCES contents(id),
    view_count bigint NOT NULL DEFAULT 0,
    like_count bigint NOT NULL DEFAULT 0,
    comment_count bigint NOT NULL DEFAULT 0,
    share_count bigint NOT NULL DEFAULT 0
);
CREATE TABLE collection_runs (
    id bigint PRIMARY KEY,
    source_connection_id bigint NOT NULL REFERENCES source_connections(id)
);
CREATE TABLE collection_run_items (
    id bigint PRIMARY KEY,
    run_id bigint NOT NULL REFERENCES collection_runs(id),
    content_id bigint REFERENCES contents(id),
    outcome varchar(16) NOT NULL
);`); err != nil {
		t.Fatalf("create PLAN-006 legacy fixture: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO source_connections (id) VALUES (10);
INSERT INTO contents (id, source_connection_id, content_status, view_count, like_count, comment_count, share_count)
VALUES (20, 10, 'active', 0, 12, 0, 4);
INSERT INTO content_metric_snapshots (id, content_id, view_count, like_count, comment_count, share_count)
VALUES (30, 20, 0, 8, 0, 2);
INSERT INTO collection_runs (id, source_connection_id) VALUES (40, 10);
INSERT INTO collection_run_items (id, run_id, outcome) VALUES (50, 40, 'captured');
INSERT INTO collection_run_items (id, run_id, content_id, outcome) VALUES (51, 40, 20, 'captured');`); err != nil {
		t.Fatalf("seed PLAN-006 legacy fixture: %v", err)
	}

	if _, err := runtime.SQL.Exec(plan007UpgradeSQL(t)); err != nil {
		t.Fatalf("apply PLAN-007 upgrade runbook: %v", err)
	}

	var itemSourceID int64
	if err := runtime.SQL.QueryRow(`SELECT source_connection_id FROM collection_run_items WHERE id = 50`).Scan(&itemSourceID); err != nil {
		t.Fatalf("read upgraded collection item source: %v", err)
	}
	if itemSourceID != 10 {
		t.Fatalf("upgraded collection item source = %d, want 10", itemSourceID)
	}
	var boundStatus, boundError string
	if err := runtime.SQL.QueryRow(`
SELECT ingestion_status, COALESCE(ingestion_error_code, '')
FROM collection_run_items
WHERE id = 51`).Scan(&boundStatus, &boundError); err != nil {
		t.Fatalf("read upgraded bound collection item state: %v", err)
	}
	if boundStatus != "succeeded" || boundError != "" {
		t.Fatalf("upgraded bound collection item state = %q/%q, want succeeded/empty", boundStatus, boundError)
	}
	if _, err := runtime.SQL.Exec(`UPDATE collection_run_items SET ingestion_status = 'succeeded' WHERE id = 50`); err == nil {
		t.Fatal("unbound captured item updated to succeeded = nil error, want ingestion state CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`UPDATE collection_run_items SET ingestion_status = 'failed', ingestion_error_code = NULL WHERE id = 50`); err == nil {
		t.Fatal("failed captured item without stable error code = nil error, want ingestion state CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`UPDATE collection_run_items SET ingestion_status = 'pending' WHERE id = 51`); err == nil {
		t.Fatal("bound captured item updated to pending = nil error, want ingestion state CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	var contentView, contentLike, contentComment, contentShare any
	if err := runtime.SQL.QueryRow(`SELECT view_count, like_count, comment_count, share_count FROM contents WHERE id = 20`).Scan(&contentView, &contentLike, &contentComment, &contentShare); err != nil {
		t.Fatalf("read upgraded content metrics: %v", err)
	}
	if contentView != nil || contentLike != int64(12) || contentComment != nil || contentShare != int64(4) {
		t.Fatalf("upgraded content metrics = %#v/%#v/%#v/%#v, want nil/12/nil/4", contentView, contentLike, contentComment, contentShare)
	}
	var snapshotView, snapshotLike, snapshotComment, snapshotShare any
	if err := runtime.SQL.QueryRow(`SELECT view_count, like_count, comment_count, share_count FROM content_metric_snapshots WHERE id = 30`).Scan(&snapshotView, &snapshotLike, &snapshotComment, &snapshotShare); err != nil {
		t.Fatalf("read upgraded snapshot metrics: %v", err)
	}
	if snapshotView != nil || snapshotLike != int64(8) || snapshotComment != nil || snapshotShare != int64(2) {
		t.Fatalf("upgraded snapshot metrics = %#v/%#v/%#v/%#v, want nil/8/nil/2", snapshotView, snapshotLike, snapshotComment, snapshotShare)
	}
	var defaultSnapshotView, defaultSnapshotLike, defaultSnapshotComment, defaultSnapshotShare any
	if err := runtime.SQL.QueryRow(`
INSERT INTO content_metric_snapshots (id, content_id)
VALUES (31, 20)
RETURNING view_count, like_count, comment_count, share_count`).Scan(&defaultSnapshotView, &defaultSnapshotLike, &defaultSnapshotComment, &defaultSnapshotShare); err != nil {
		t.Fatalf("insert upgraded snapshot without metrics: %v", err)
	}
	if defaultSnapshotView != nil || defaultSnapshotLike != nil || defaultSnapshotComment != nil || defaultSnapshotShare != nil {
		t.Fatalf("upgraded snapshot defaults = %#v/%#v/%#v/%#v, want all nil", defaultSnapshotView, defaultSnapshotLike, defaultSnapshotComment, defaultSnapshotShare)
	}
}

func TestPlan007SchemaUpgradeRestoresCanonicalCatalogFromLegacyLayout(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	if _, err := runtime.SQL.Exec(plan007LegacyLayoutSQL); err != nil {
		t.Fatalf("restore PLAN-006 catalog layout: %v", err)
	}
	if _, err := runtime.SQL.Exec(plan007UpgradeSQL(t)); err != nil {
		t.Fatalf("apply PLAN-007 upgrade runbook to legacy layout: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err != nil {
		t.Fatalf("verify upgraded PLAN-006 catalog against canonical schema: %v", err)
	}
}

func TestPlan007SchemaUpgradeRollbackRestoresLegacyBackup(t *testing.T) {
	t.Run("legacy restore fails before PLAN-007 foreign-key cleanup", func(t *testing.T) {
		runtime, backup := plan007UpgradedLegacyRuntime(t)
		output, err := runPostgreSQLTool(t, "pg_restore", "--clean", "--if-exists", "--no-owner", "--dbname="+runtime.Pool.Config().ConnString(), backup)
		if err == nil {
			t.Fatal("restore legacy backup without PLAN-007 foreign-key cleanup = nil error, want dependency failure")
		}
		if !strings.Contains(output, "cannot drop table") || !strings.Contains(output, "collection_run_items_content_source_connection_fkey") {
			t.Fatalf("restore legacy backup failure did not identify the PLAN-007 content foreign key: %s", output)
		}
	})

	t.Run("runbook foreign-key cleanup restores legacy schema and rows", func(t *testing.T) {
		runtime, backup := plan007UpgradedLegacyRuntime(t)
		if _, err := runtime.SQL.Exec(plan007RollbackForeignKeyCleanupSQL(t)); err != nil {
			t.Fatalf("run PLAN-007 rollback foreign-key cleanup: %v", err)
		}
		if output, err := runPostgreSQLTool(t, "pg_restore", "--clean", "--if-exists", "--no-owner", "--dbname="+runtime.Pool.Config().ConnString(), backup); err != nil {
			t.Fatalf("restore legacy backup after PLAN-007 foreign-key cleanup: %v\n%s", err, output)
		}
		assertPlan007LegacyBackupRestored(t, runtime)
	})
}

func TestPlan008SchemaUpgradeAndRollbackUsesPinnedPlan007Worktree(t *testing.T) {
	worktree := plan007Worktree(t)
	targetWorktree := plan008Worktree(t)
	dsn := postgresfixture.New(t)

	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "init", "--empty-only", "--confirm-empty"); err != nil {
		t.Fatalf("initialize detached PLAN-007 worktree database: %v\n%s", err, output)
	}
	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "verify"); err != nil {
		t.Fatalf("verify detached PLAN-007 worktree database: %v\n%s", err, output)
	}

	backup := filepath.Join(t.TempDir(), "plan008-plan007-before-upgrade.dump")
	if output, err := runPostgreSQLTool(t, "pg_dump", dsn, "--format=custom", "--file="+backup); err != nil {
		t.Fatalf("dump PLAN-007 database: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan008UpgradeSQL(t)); err != nil {
		t.Fatalf("apply exact PLAN-008 upgrade runbook: %v\n%s", err, output)
	}

	if output, err := runHistoricalDatabaseCommand(targetWorktree, dsn, "verify"); err != nil {
		t.Fatalf("verify upgraded database with detached PLAN-008 worktree: %v\n%s", err, output)
	}
	current, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open upgraded PLAN-008 database: %v", err)
	}
	var aiRows int
	if err := current.SQL.QueryRow(`
SELECT
  (SELECT count(*) FROM ai_model_profiles) +
  (SELECT count(*) FROM ai_runs) +
  (SELECT count(*) FROM ai_run_evidences) +
  (SELECT count(*) FROM content_embeddings) +
  (SELECT count(*) FROM monitor_embeddings) +
  (SELECT count(*) FROM event_embeddings) +
  (SELECT count(*) FROM topic_embeddings)`).Scan(&aiRows); err != nil {
		_ = current.Close()
		t.Fatalf("count upgraded AI rows: %v", err)
	}
	if aiRows != 0 {
		_ = current.Close()
		t.Fatalf("upgraded PLAN-008 database has %d legacy AI rows, want 0", aiRows)
	}
	if err := current.Close(); err != nil {
		t.Fatalf("close upgraded PLAN-008 database: %v", err)
	}

	if output, err := runPostgreSQLTool(t, "pg_restore", "--single-transaction", "--clean", "--if-exists", "--no-owner", "--dbname="+dsn, backup); err == nil {
		t.Fatal("unprepared PLAN-008 restore succeeded, want the documented ledger dependency failure")
	} else if output == "" {
		t.Fatal("unprepared PLAN-008 restore failed without diagnostic output")
	}

	if output, err := runHistoricalDatabaseCommand(targetWorktree, dsn, "verify"); err != nil {
		t.Fatalf("unprepared restore was not atomic: %v\n%s", err, output)
	}

	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan008RollbackPreparationSQL(t)); err != nil {
		t.Fatalf("apply exact PLAN-008 rollback preparation: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "pg_restore", "--single-transaction", "--clean", "--if-exists", "--no-owner", "--dbname="+dsn, backup); err != nil {
		t.Fatalf("restore PLAN-007 backup after PLAN-008 preparation: %v\n%s", err, output)
	}
	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "verify"); err != nil {
		t.Fatalf("verify restored database with detached PLAN-007 worktree: %v\n%s", err, output)
	}
}

func TestPlan009SchemaUpgradeAndRollbackUsesPinnedPlan008Worktree(t *testing.T) {
	worktree := plan008Worktree(t)
	targetWorktree := plan009Worktree(t)
	dsn := postgresfixture.New(t)

	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "init", "--empty-only", "--confirm-empty"); err != nil {
		t.Fatalf("initialize detached PLAN-008 worktree database: %v\n%s", err, output)
	}
	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "verify"); err != nil {
		t.Fatalf("verify detached PLAN-008 worktree database: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan009MonitorMatchFixtureSQL); err != nil {
		t.Fatalf("seed non-empty PLAN-008 monitor-match history: %v\n%s", err, output)
	}

	backup := filepath.Join(t.TempDir(), "plan009-plan008-before-upgrade.dump")
	if output, err := runPostgreSQLTool(t, "pg_dump", dsn, "--format=custom", "--file="+backup); err != nil {
		t.Fatalf("dump PLAN-008 database: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan009UpgradeSQL(t)); err != nil {
		t.Fatalf("apply exact PLAN-009 upgrade runbook: %v\n%s", err, output)
	}

	current, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open upgraded PLAN-009 database: %v", err)
	}
	if output, err := runHistoricalDatabaseCommand(targetWorktree, dsn, "verify"); err != nil {
		_ = current.Close()
		t.Fatalf("verify upgraded PLAN-009 catalog with pinned PLAN-009 worktree: %v\n%s", err, output)
	}
	var historicalMatches int
	if err := current.SQL.QueryRow(`SELECT count(*) FROM monitor_matches WHERE algorithm_version = 'plan009-upgrade-fixture'`).Scan(&historicalMatches); err != nil {
		_ = current.Close()
		t.Fatalf("count upgraded historical monitor matches: %v", err)
	}
	if historicalMatches != 1 {
		_ = current.Close()
		t.Fatalf("upgraded historical monitor matches = %d, want 1", historicalMatches)
	}
	var inputHash, scoringVersion, decisionOrigin string
	var degraded, recallPathsEmpty, legacyBackfill, deterministicHash bool
	if err := current.SQL.QueryRow(`
SELECT input_hash, scoring_version, decision_origin, degraded,
       recall_paths = ARRAY[]::text[],
       COALESCE((explanation->'provenance'->>'legacy_backfill')::boolean, false),
       input_hash = encode(
         sha256(convert_to(concat_ws(':', 'legacy-monitor-match-v1', id, monitor_config_version_id, content_id, algorithm_version), 'UTF8')),
         'hex'
       )
FROM monitor_matches
WHERE algorithm_version = 'plan009-upgrade-fixture'`).Scan(
		&inputHash, &scoringVersion, &decisionOrigin, &degraded, &recallPathsEmpty, &legacyBackfill, &deterministicHash,
	); err != nil {
		_ = current.Close()
		t.Fatalf("read upgraded historical monitor-match provenance: %v", err)
	}
	if len(inputHash) != 64 || scoringVersion != "legacy-v1" || decisionOrigin != "rule" || !degraded || !recallPathsEmpty || !legacyBackfill || !deterministicHash {
		_ = current.Close()
		t.Fatalf("upgraded historical monitor-match provenance = hash=%q version=%q origin=%q degraded=%t empty_paths=%t legacy=%t deterministic=%t", inputHash, scoringVersion, decisionOrigin, degraded, recallPathsEmpty, legacyBackfill, deterministicHash)
	}
	if _, err := current.SQL.Exec(`
INSERT INTO ai_model_profiles (
  name, task_type, provider, model_name, credential_ref, model_version,
  max_attempts, max_cost, daily_budget
) VALUES (
  'plan009-relevance-review', 'relevance_review', 'openai', 'gpt-5.6sol', 'env:OPENAI_API_KEY', '2026-07',
  2, 0.1000, 1.0000
)`); err != nil {
		_ = current.Close()
		t.Fatalf("insert upgraded relevance-review profile: %v", err)
	}
	if _, err := current.SQL.Exec(`
INSERT INTO ai_model_profiles (
  name, task_type, provider, model_name, credential_ref, model_version,
  embedding_dimensions, max_attempts, max_cost
) VALUES (
  'plan009-invalid-relevance-review', 'relevance_review', 'openai', 'gpt-5.6sol', 'env:OPENAI_API_KEY', '2026-07',
  1024, 2, 0.1000
)`); err == nil {
		_ = current.Close()
		t.Fatal("insert relevance-review profile with embedding dimensions error = nil, want CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if err := current.Close(); err != nil {
		t.Fatalf("close upgraded PLAN-009 database: %v", err)
	}

	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan009RollbackPreparationSQL(t)); err != nil {
		t.Fatalf("apply exact PLAN-009 rollback preparation: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "pg_restore", "--single-transaction", "--clean", "--if-exists", "--no-owner", "--dbname="+dsn, backup); err != nil {
		t.Fatalf("restore PLAN-008 backup after PLAN-009 upgrade: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan009RollbackNormalizationSQL(t)); err != nil {
		t.Fatalf("normalize restored PLAN-008 index catalog: %v\n%s", err, output)
	}
	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "verify"); err != nil {
		t.Fatalf("verify restored database with detached PLAN-008 worktree: %v\n%s", err, output)
	}
	restored, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open restored PLAN-008 database: %v", err)
	}
	var restoredMatches int
	if err := restored.SQL.QueryRow(`
SELECT count(*)
FROM monitor_matches
WHERE algorithm_version = 'plan009-upgrade-fixture'
  AND rule_score = 70 AND final_score = 70 AND decision = 'review'
  AND reason_codes = ARRAY['fixture']::text[]`).Scan(&restoredMatches); err != nil {
		_ = restored.Close()
		t.Fatalf("verify restored monitor-match history: %v", err)
	}
	if err := restored.Close(); err != nil {
		t.Fatalf("close restored PLAN-008 database: %v", err)
	}
	if restoredMatches != 1 {
		t.Fatalf("restored historical monitor matches = %d, want 1", restoredMatches)
	}
}

func TestPlan009SchemaUpgradeRejectsLegacyScalarProvenance(t *testing.T) {
	worktree := plan008Worktree(t)
	dsn := postgresfixture.New(t)

	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "init", "--empty-only", "--confirm-empty"); err != nil {
		t.Fatalf("initialize detached PLAN-008 worktree database: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan009MonitorMatchFixtureSQL); err != nil {
		t.Fatalf("seed PLAN-008 monitor-match history: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", `UPDATE monitor_matches SET explanation = '{"provenance":"legacy"}'::jsonb`); err != nil {
		t.Fatalf("seed scalar legacy provenance: %v\n%s", err, output)
	}
	if output, err := runPostgreSQLTool(t, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-c", plan009UpgradeSQL(t)); err == nil {
		t.Fatal("upgrade with scalar legacy provenance succeeded, want preflight rejection")
	} else if !strings.Contains(output, "requires monitor_matches explanation and provenance to be JSON objects") {
		t.Fatalf("upgrade with scalar legacy provenance failed without expected diagnostic: %s", output)
	}
	if output, err := runHistoricalDatabaseCommand(worktree, dsn, "verify"); err != nil {
		t.Fatalf("failed PLAN-009 preflight must leave the PLAN-008 catalog unchanged: %v\n%s", err, output)
	}
}

func plan007UpgradedLegacyRuntime(t *testing.T) (*Runtime, string) {
	t.Helper()
	runtime := openTestRuntime(t)
	t.Cleanup(func() { _ = runtime.Close() })

	if _, err := runtime.SQL.Exec(plan007LegacyLayoutSQL); err != nil {
		t.Fatalf("restore PLAN-006 catalog layout: %v", err)
	}
	var sourceID, contentID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', 'plan007-rollback-source', 'https://rollback.example.test/feed')
RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("seed PLAN-006 source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (
    source_connection_id, external_id, content_type, title, canonical_url, language,
    published_at, fetched_at, dedupe_key, view_count, like_count, comment_count, share_count
)
VALUES ($1, 'plan007-rollback-content', 'article', 'Legacy rollback content',
        'https://rollback.example.test/content', 'en', now(), now(), $2, 0, 12, 0, 4)
RETURNING id`, sourceID, strings.Repeat("a", 64)).Scan(&contentID); err != nil {
		t.Fatalf("seed PLAN-006 content: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO content_metric_snapshots (content_id, captured_at, view_count, like_count, comment_count, share_count)
VALUES ($1, now(), 0, 8, 0, 2)`, contentID); err != nil {
		t.Fatalf("seed PLAN-006 metric snapshot: %v", err)
	}
	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at)
VALUES ($1, $2, now(), now() + interval '1 minute', 'manual', now())
RETURNING id`, sourceID, strings.Repeat("b", 64)).Scan(&runID); err != nil {
		t.Fatalf("seed PLAN-006 collection run: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_items (
    run_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, 'rss', 'plan007-rollback-item', 'article', 'v1', '{"title":"legacy"}'::jsonb,
        $2, 'discarded', 'captured', now())`, runID, strings.Repeat("c", 64)); err != nil {
		t.Fatalf("seed PLAN-006 captured item: %v", err)
	}

	backup := filepath.Join(t.TempDir(), "plan007-legacy.dump")
	if output, err := runPostgreSQLTool(t, "pg_dump", runtime.Pool.Config().ConnString(), "--format=custom", "--file="+backup); err != nil {
		t.Fatalf("dump PLAN-006 legacy database: %v\n%s", err, output)
	}
	if _, err := runtime.SQL.Exec(plan007UpgradeSQL(t)); err != nil {
		t.Fatalf("apply PLAN-007 upgrade runbook: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err != nil {
		t.Fatalf("verify upgraded PLAN-006 catalog against canonical schema: %v", err)
	}
	return runtime, backup
}

func runPostgreSQLTool(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is required for PLAN-007 rollback integration: %v", name, err)
	}
	output, err := exec.Command(name, args...).CombinedOutput()
	return string(output), err
}

func plan007Worktree(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	worktree := filepath.Join(t.TempDir(), "plan007-worktree")
	command := exec.Command("git", "-C", root, "worktree", "add", "--detach", worktree, "53d7f01")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create detached PLAN-007 worktree: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		output, err := exec.Command("git", "-C", root, "worktree", "remove", "--force", worktree).CombinedOutput()
		if err != nil {
			t.Errorf("remove detached PLAN-007 worktree: %v\n%s", err, output)
		}
	})
	return worktree
}

func plan008Worktree(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	worktree := filepath.Join(t.TempDir(), "plan008-worktree")
	command := exec.Command("git", "-C", root, "worktree", "add", "--detach", worktree, "a7fc805")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create detached PLAN-008 worktree: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		output, err := exec.Command("git", "-C", root, "worktree", "remove", "--force", worktree).CombinedOutput()
		if err != nil {
			t.Errorf("remove detached PLAN-008 worktree: %v\n%s", err, output)
		}
	})
	return worktree
}

func plan009Worktree(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	worktree := filepath.Join(t.TempDir(), "plan009-worktree")
	command := exec.Command("git", "-C", root, "worktree", "add", "--detach", worktree, "7cb8148")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create detached PLAN-009 worktree: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("git", "-C", root, "worktree", "remove", "--force", worktree).CombinedOutput(); err != nil {
			t.Errorf("remove detached PLAN-009 worktree: %v\n%s", err, output)
		}
	})
	return worktree
}

func runHistoricalDatabaseCommand(worktree, dsn string, args ...string) (string, error) {
	command := exec.Command("go", append([]string{"-C", worktree, "run", "./cmd/hotkey", "db"}, args...)...)
	command.Env = append(os.Environ(), "HOTKEY_DATABASE_URL="+dsn)
	output, err := command.CombinedOutput()
	return string(output), err
}

func assertPlan007LegacyBackupRestored(t *testing.T, runtime *Runtime) {
	t.Helper()
	var legacyContent, legacySnapshot, dedupeColumns, sourceColumns, plan007ForeignKeys int
	if err := runtime.SQL.QueryRow(`
SELECT
  (SELECT count(*) FROM contents
   WHERE external_id = 'plan007-rollback-content'
     AND view_count = 0 AND like_count = 12 AND comment_count = 0 AND share_count = 4),
  (SELECT count(*) FROM content_metric_snapshots
   WHERE view_count = 0 AND like_count = 8 AND comment_count = 0 AND share_count = 2),
  (SELECT count(*) FROM information_schema.columns
   WHERE table_schema = 'public' AND table_name = 'contents'
     AND column_name IN ('dedupe_reason', 'dedupe_version')),
  (SELECT count(*) FROM information_schema.columns
   WHERE table_schema = 'public' AND table_name = 'collection_run_items'
     AND column_name = 'source_connection_id'),
  (SELECT count(*) FROM pg_constraint
   WHERE conname IN ('collection_run_items_run_source_connection_fkey', 'collection_run_items_content_source_connection_fkey'))`).
		Scan(&legacyContent, &legacySnapshot, &dedupeColumns, &sourceColumns, &plan007ForeignKeys); err != nil {
		t.Fatalf("read restored PLAN-006 facts: %v", err)
	}
	if legacyContent != 1 || legacySnapshot != 1 || dedupeColumns != 0 || sourceColumns != 0 || plan007ForeignKeys != 0 {
		t.Fatalf("restored PLAN-006 facts = content=%d snapshot=%d dedupe-columns=%d source-columns=%d plan007-foreign-keys=%d, want 1/1/0/0/0", legacyContent, legacySnapshot, dedupeColumns, sourceColumns, plan007ForeignKeys)
	}
}

const plan007LegacyLayoutSQL = `
ALTER TABLE collection_run_items
  DROP CONSTRAINT collection_run_items_run_id_source_connection_id_fkey,
  DROP CONSTRAINT collection_run_items_content_id_source_connection_id_fkey,
  DROP CONSTRAINT collection_run_items_check,
  DROP COLUMN source_connection_id,
  DROP COLUMN ingestion_status,
  DROP COLUMN ingestion_error_code,
  ADD CONSTRAINT collection_run_items_content_id_fkey
    FOREIGN KEY (content_id) REFERENCES contents(id) ON DELETE SET NULL;

ALTER TABLE contents
  DROP CONSTRAINT contents_check,
  DROP CONSTRAINT contents_id_source_connection_id_key,
  DROP COLUMN dedupe_reason,
  DROP COLUMN dedupe_version,
  ALTER COLUMN view_count SET DEFAULT 0,
  ALTER COLUMN view_count SET NOT NULL,
  ALTER COLUMN like_count SET DEFAULT 0,
  ALTER COLUMN like_count SET NOT NULL,
  ALTER COLUMN comment_count SET DEFAULT 0,
  ALTER COLUMN comment_count SET NOT NULL,
  ALTER COLUMN share_count SET DEFAULT 0,
  ALTER COLUMN share_count SET NOT NULL;

ALTER TABLE content_metric_snapshots
  ALTER COLUMN view_count SET DEFAULT 0,
  ALTER COLUMN view_count SET NOT NULL,
  ALTER COLUMN like_count SET DEFAULT 0,
  ALTER COLUMN like_count SET NOT NULL,
  ALTER COLUMN comment_count SET DEFAULT 0,
  ALTER COLUMN comment_count SET NOT NULL,
  ALTER COLUMN share_count SET DEFAULT 0,
  ALTER COLUMN share_count SET NOT NULL;

ALTER TABLE collection_runs
  DROP CONSTRAINT collection_runs_id_source_connection_id_key;
`

func plan007UpgradeSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "operations", "plan007-schema-upgrade.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN-007 upgrade runbook: %v", err)
	}
	text := string(content)
	start := strings.Index(text, "BEGIN;\n")
	if start < 0 {
		t.Fatal("PLAN-007 upgrade runbook has no transaction block")
	}
	end := strings.Index(text[start:], "\nCOMMIT;")
	if end < 0 {
		t.Fatal("PLAN-007 upgrade runbook has no COMMIT")
	}
	return text[start : start+end+len("\nCOMMIT;")]
}

func plan008UpgradeSQL(t *testing.T) string {
	t.Helper()
	return plan008RunbookTransaction(t, "BEGIN;\n\nALTER TABLE ai_model_profiles", "PLAN-008 upgrade")
}

func plan009UpgradeSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "operations", "plan009-schema-upgrade.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN-009 upgrade runbook: %v", err)
	}
	text := string(content)
	start := strings.Index(text, "BEGIN;\n\nALTER TABLE ai_model_profiles")
	if start < 0 {
		t.Fatal("PLAN-009 upgrade runbook transaction is missing")
	}
	end := strings.Index(text[start:], "\nCOMMIT;")
	if end < 0 {
		t.Fatal("PLAN-009 upgrade runbook transaction has no COMMIT")
	}
	return text[start : start+end+len("\nCOMMIT;")]
}

func plan009RollbackNormalizationSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "operations", "plan009-schema-upgrade.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN-009 rollback runbook: %v", err)
	}
	text := string(content)
	start := strings.Index(text, "BEGIN;\n\nDROP INDEX IF EXISTS ai_runs_reuse_inflight_uq")
	if start < 0 {
		t.Fatal("PLAN-009 rollback normalization transaction is missing")
	}
	end := strings.Index(text[start:], "\nCOMMIT;")
	if end < 0 {
		t.Fatal("PLAN-009 rollback normalization transaction has no COMMIT")
	}
	return text[start : start+end+len("\nCOMMIT;")]
}

func plan009RollbackPreparationSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "operations", "plan009-schema-upgrade.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN-009 rollback runbook: %v", err)
	}
	text := string(content)
	start := strings.Index(text, "BEGIN;\n\nDROP TABLE IF EXISTS monitor_match_feedbacks")
	if start < 0 {
		t.Fatal("PLAN-009 rollback preparation transaction is missing")
	}
	end := strings.Index(text[start:], "\nCOMMIT;")
	if end < 0 {
		t.Fatal("PLAN-009 rollback preparation transaction has no COMMIT")
	}
	return text[start : start+end+len("\nCOMMIT;")]
}

const plan009MonitorMatchFixtureSQL = `
WITH source AS (
  INSERT INTO source_connections (source_type, name, endpoint)
  VALUES ('rss', 'plan009-upgrade-source', 'https://plan009.example.test/feed')
  RETURNING id
), monitor AS (
  INSERT INTO monitors (name)
  VALUES ('plan009-upgrade-monitor')
  RETURNING id
), config AS (
  INSERT INTO monitor_config_versions (monitor_id, revision)
  SELECT id, 1 FROM monitor
  RETURNING id, monitor_id
), content AS (
  INSERT INTO contents (
    source_connection_id, external_id, content_type, title, canonical_url, published_at, fetched_at, dedupe_key
  )
  SELECT id, 'plan009-upgrade-content', 'article', 'Historical relevance fixture',
         'https://plan009.example.test/content', now(), now(), repeat('a', 64)
  FROM source
  RETURNING id
)
INSERT INTO monitor_matches (
  monitor_id, monitor_config_version_id, content_id, rule_score, final_score, decision, reason_codes, algorithm_version
)
SELECT monitor.id, config.id, content.id, 70, 70, 'review', ARRAY['fixture'], 'plan009-upgrade-fixture'
FROM monitor, config, content;`

func plan008RollbackPreparationSQL(t *testing.T) string {
	t.Helper()
	return plan008RunbookTransaction(t, "BEGIN;\nALTER TABLE content_embeddings DROP CONSTRAINT IF EXISTS content_embeddings_ai_run_id_fkey", "PLAN-008 rollback preparation")
}

func plan008RunbookTransaction(t *testing.T, opening, description string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "operations", "plan008-schema-upgrade.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s runbook: %v", description, err)
	}
	text := string(content)
	start := strings.Index(text, opening)
	if start < 0 {
		t.Fatalf("%s runbook transaction is missing", description)
	}
	end := strings.Index(text[start:], "\nCOMMIT;")
	if end < 0 {
		t.Fatalf("%s runbook transaction has no COMMIT", description)
	}
	return text[start : start+end+len("\nCOMMIT;")]
}

func plan007RollbackForeignKeyCleanupSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "operations", "plan007-schema-upgrade.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN-007 upgrade runbook: %v", err)
	}
	text := string(content)
	start := strings.Index(text, "ALTER TABLE collection_run_items\n  DROP CONSTRAINT IF EXISTS collection_run_items_run_source_connection_fkey")
	if start < 0 {
		t.Fatal("PLAN-007 upgrade runbook has no pre-restore PLAN-007 foreign-key cleanup")
	}
	end := strings.Index(text[start:], ";\n")
	if end < 0 {
		t.Fatal("PLAN-007 upgrade runbook foreign-key cleanup has no statement terminator")
	}
	return text[start : start+end+1]
}

func assertMonitorPointerForeignKey(t *testing.T, runtime *Runtime, name string) {
	t.Helper()
	var deleteAction string
	err := runtime.SQL.QueryRow(`
SELECT con.confdeltype::text
FROM pg_constraint AS con
WHERE con.conname = $1
  AND con.conrelid = 'monitors'::regclass
  AND con.confrelid = 'monitor_config_versions'::regclass
  AND con.contype = 'f'`, name).Scan(&deleteAction)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if deleteAction != "r" {
		t.Fatalf("%s delete action = %q, want r (RESTRICT)", name, deleteAction)
	}
}

func assertPostgreSQLRestrictViolation(t *testing.T, err error) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		t.Fatalf("database error = %T %v, want PostgreSQL RESTRICT violation", err, err)
	}
	// The catalog assertion proves this foreign key uses RESTRICT. PostgreSQL
	// 16 and 18 report the rejected delete with different, valid SQLSTATEs.
	if postgresError.Code != "23001" && postgresError.Code != "23503" {
		t.Fatalf("database SQLSTATE = %s, want 23001 or 23503 for ON DELETE RESTRICT: %v", postgresError.Code, err)
	}
}

func assertPostgreSQLState(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("database error = nil, want SQLSTATE %s", want)
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		t.Fatalf("database error = %T %v, want PostgreSQL SQLSTATE %s", err, err, want)
	}
	if postgresError.Code != want {
		t.Fatalf("database SQLSTATE = %s, want %s: %v", postgresError.Code, want, err)
	}
}

func TestInitializeEmptyRejectsExistingSchema(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	if err := InitializeEmpty(context.Background(), runtime.Pool); err == nil {
		t.Fatal("InitializeEmpty() error = nil, want non-empty schema rejection")
	}
}

func TestInitializeEmptyRejectsExistingPublicView(t *testing.T) {
	runtime := openEmptyTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	if _, err := runtime.SQL.Exec("CREATE VIEW existing_public_view AS SELECT 1 AS id"); err != nil {
		t.Fatalf("create public view: %v", err)
	}
	if err := InitializeEmpty(context.Background(), runtime.Pool); err == nil {
		t.Fatal("InitializeEmpty() error = nil, want existing public view rejection")
	}
}

func TestInitializeEmptyRejectsExistingPublicCompositeType(t *testing.T) {
	runtime := openEmptyTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	if _, err := runtime.SQL.Exec("CREATE TYPE existing_public_composite AS (id bigint)"); err != nil {
		t.Fatalf("create public composite type: %v", err)
	}
	if err := InitializeEmpty(context.Background(), runtime.Pool); err == nil {
		t.Fatal("InitializeEmpty() error = nil, want existing public composite type rejection")
	}
}

func TestTransactionsCommitRollbackPanicAndCancellation(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	ctx := context.Background()
	prefix := fmt.Sprintf("database-tx-%d", time.Now().UnixNano())
	commitEmail := prefix + "-commit@example.test"
	rollbackEmail := prefix + "-rollback@example.test"
	panicEmail := prefix + "-panic@example.test"
	defer func() {
		_, _ = runtime.SQL.Exec("DELETE FROM users WHERE email LIKE $1", prefix+"%")
	}()
	if err := runtime.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		if _, err := tx.SQL.ExecContext(ctx, userInsertSQL, commitEmail, "commit"); err != nil {
			return err
		}
		result := tx.GORM.Exec("UPDATE users SET display_name = ? WHERE email = ?", "committed", commitEmail)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("GORM transaction update affected %d rows, want 1", result.RowsAffected)
		}
		return nil
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	assertUserCount(t, runtime, commitEmail, 1)
	var displayName string
	if err := runtime.SQL.QueryRow("SELECT display_name FROM users WHERE email = $1", commitEmail).Scan(&displayName); err != nil {
		t.Fatalf("read committed GORM update: %v", err)
	}
	if displayName != "committed" {
		t.Fatalf("committed display name = %q, want committed", displayName)
	}

	wantRollback := errors.New("rollback")
	if err := runtime.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		if _, err := tx.SQL.ExecContext(ctx, userInsertSQL, rollbackEmail, "rollback"); err != nil {
			return err
		}
		return wantRollback
	}); !errors.Is(err, wantRollback) {
		t.Fatalf("rollback transaction error = %v, want %v", err, wantRollback)
	}
	assertUserCount(t, runtime, rollbackEmail, 0)

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("panic transaction did not re-panic")
			}
		}()
		_ = runtime.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
			if _, err := tx.SQL.ExecContext(ctx, userInsertSQL, panicEmail, "panic"); err != nil {
				return err
			}
			panic("intentional transaction panic")
		})
	}()
	assertUserCount(t, runtime, panicEmail, 0)

	canceled, cancel := context.WithCancel(ctx)
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- runtime.WithinTransaction(canceled, func(ctx context.Context, tx Transaction) error {
			close(started)
			_, err := tx.SQL.ExecContext(ctx, "SELECT pg_sleep(5)")
			return err
		})
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation test did not start its transaction")
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("active canceled transaction error = nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active canceled transaction did not return promptly")
	}
	if err := runtime.Ping(context.Background()); err != nil {
		t.Fatalf("pool did not recover after canceled transaction: %v", err)
	}
}

func TestWithinTransactionRejectsNestedTransactions(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()

	outer := context.Background()
	err := runtime.WithinTransaction(outer, func(ctx context.Context, _ Transaction) error {
		if err := runtime.WithinTransaction(ctx, func(context.Context, Transaction) error { return nil }); !errors.Is(err, ErrNestedTransaction) {
			return fmt.Errorf("callback context nested transaction = %v, want ErrNestedTransaction", err)
		}
		return runtime.WithinTransaction(outer, func(context.Context, Transaction) error { return nil })
	})
	if err != nil {
		t.Fatalf("outer context starts independent transaction: %v", err)
	}
}

const userInsertSQL = `
INSERT INTO users (email, password_hash, display_name, role)
VALUES ($1, 'hash', $2, 'viewer')`

func openTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime := openEmptyTestRuntime(t)
	if err := InitializeEmpty(context.Background(), runtime.Pool); err != nil {
		t.Fatalf("InitializeEmpty() error = %v", err)
	}
	return runtime
}

func openEmptyTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return runtime
}

func assertUserCount(t *testing.T, runtime *Runtime, email string, want int) {
	t.Helper()
	var got int
	if err := runtime.SQL.QueryRow("SELECT count(*) FROM users WHERE email = $1", email).Scan(&got); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if got != want {
		t.Fatalf("count users for %q = %d, want %d", email, got, want)
	}
}

func TestVerifyRejectsUnexpectedPublicTable(t *testing.T) {
	runtime := openTestRuntime(t)

	name := fmt.Sprintf("plan002_verify_probe_%d", time.Now().UnixNano())
	defer func() {
		_, _ = runtime.SQL.Exec("DROP TABLE IF EXISTS " + name)
		_ = runtime.Close()
	}()
	if _, err := runtime.SQL.Exec("CREATE TABLE " + name + " (id bigint PRIMARY KEY)"); err != nil {
		t.Fatalf("create unexpected table: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want unexpected table rejection")
	}
}

func TestVerifyRejectsMissingCheckConstraint(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	defer func() {
		_, _ = runtime.SQL.Exec(`
ALTER TABLE monitor_config_versions
ADD CONSTRAINT monitor_config_versions_relevance_threshold_check
CHECK (relevance_threshold BETWEEN 60 AND 100)`)
	}()

	if _, err := runtime.SQL.Exec("ALTER TABLE monitor_config_versions DROP CONSTRAINT monitor_config_versions_relevance_threshold_check"); err != nil {
		t.Fatalf("drop check constraint: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want missing check constraint rejection")
	}
}

func TestVerifyRejectsChangedCheckConstraintDefinition(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	if _, err := runtime.SQL.Exec(`
ALTER TABLE monitor_config_versions DROP CONSTRAINT monitor_config_versions_relevance_threshold_check;
ALTER TABLE monitor_config_versions ADD CONSTRAINT monitor_config_versions_relevance_threshold_check
CHECK (relevance_threshold BETWEEN -100 AND 100)`); err != nil {
		t.Fatalf("replace check constraint: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want changed check definition rejection")
	}
}

func TestVerifyRejectsMissingCanonicalIndex(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	defer func() {
		_, _ = runtime.SQL.Exec("CREATE INDEX contents_source_published_idx ON contents(source_connection_id, published_at DESC, id DESC)")
	}()

	if _, err := runtime.SQL.Exec("DROP INDEX contents_source_published_idx"); err != nil {
		t.Fatalf("drop canonical index: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want missing canonical index rejection")
	}
}

func TestVerifyRejectsChangedCanonicalIndexDefinition(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	if _, err := runtime.SQL.Exec(`
DROP INDEX contents_source_published_idx;
CREATE INDEX contents_source_published_idx ON contents(source_connection_id, id)`); err != nil {
		t.Fatalf("replace canonical index: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want changed index definition rejection")
	}
}

func TestVerifyRejectsChangedColumnDefault(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	if _, err := runtime.SQL.Exec("ALTER TABLE monitor_config_versions ALTER COLUMN retention_days SET DEFAULT 365"); err != nil {
		t.Fatalf("replace column default: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want changed default rejection")
	}
}
