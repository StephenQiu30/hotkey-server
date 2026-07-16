package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestEmbeddedSchemaCatalogIsComplete(t *testing.T) {
	tables := EmbeddedSchemaTableNames()
	if got, want := len(tables), 57; got != want {
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
	if got, want := len(verification.Tables), 57; got != want {
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
INSERT INTO monitor_matches (monitor_id, monitor_config_version_id, content_id, rule_score, final_score, decision, algorithm_version)
VALUES ($1, $2, $3, 10, 10, 'accepted', 'schema-test')`, secondMonitorID, firstConfigID, contentID); err == nil {
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
    run_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, 'rss', 'item-1', 'article', 'v1', '{"title":"safe"}'::jsonb, $2, 'discarded', 'captured', $3)
RETURNING id`, runID, strings.Repeat("e", 64), now).Scan(&itemID); err != nil {
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
    run_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, 'rss', 'item-2', 'article', 'v1', '{"title":"other"}'::jsonb, $2, 'discarded', 'captured', $3)
RETURNING id`, otherRunID, strings.Repeat("c", 64), now).Scan(&otherItemID); err != nil {
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
    run_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, 'rss', 'item-1', 'article', 'v1', '{"title":"safe"}'::jsonb, $2, 'discarded', 'captured', $3)`, runID, strings.Repeat("e", 64), now); err == nil {
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
    run_id, source_code, external_id, content_type, captured_item_version, captured_item,
    payload_hash, raw_payload_disposition, outcome, observed_at
)
VALUES ($1, 'rss', 'item-invalid', 'article', 'v1', '{"title":"safe"}'::jsonb, $2, 'raw_response', 'captured', $3)`, runID, strings.Repeat("a", 64), now); err == nil {
		t.Fatal("unapproved raw payload disposition = nil error, want CHECK rejection")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
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
