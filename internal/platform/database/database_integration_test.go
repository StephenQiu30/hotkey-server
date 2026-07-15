package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestEmbeddedSchemaCatalogIsComplete(t *testing.T) {
	tables := EmbeddedSchemaTableNames()
	if got, want := len(tables), 54; got != want {
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
	if got, want := len(verification.Tables), 54; got != want {
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

	now := time.Now().UTC().Round(0)
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
ALTER TABLE monitors
ADD CONSTRAINT monitors_relevance_threshold_check
CHECK (relevance_threshold BETWEEN 0 AND 100)`)
	}()

	if _, err := runtime.SQL.Exec("ALTER TABLE monitors DROP CONSTRAINT monitors_relevance_threshold_check"); err != nil {
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
ALTER TABLE monitors DROP CONSTRAINT monitors_relevance_threshold_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_relevance_threshold_check
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
	if _, err := runtime.SQL.Exec("ALTER TABLE monitors ALTER COLUMN retention_days SET DEFAULT 365"); err != nil {
		t.Fatalf("replace column default: %v", err)
	}
	if _, err := Verify(context.Background(), runtime.Pool); err == nil {
		t.Fatal("Verify() error = nil, want changed default rejection")
	}
}
