//go:build integration

package database

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDatabaseCredentialRotationPrechecksRollsBackAndRevokesOldLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminDSN := postgresfixture.New(t)
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("open isolated database administrator: %v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE TABLE credential_rotation_probe (id bigint PRIMARY KEY, marker text NOT NULL)`); err != nil {
		t.Fatalf("create credential rotation probe: %v", err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO credential_rotation_probe (id, marker) VALUES (1, 'preserved')`); err != nil {
		t.Fatalf("seed credential rotation probe: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	oldRole := "hotkey_rotation_old_" + suffix
	newRole := "hotkey_rotation_new_" + suffix
	oldSecret := "synthetic-database-old-credential-0123456789"
	newSecret := "synthetic-database-new-credential-0123456789"
	invalidSecret := "synthetic-database-invalid-credential-012345"
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, role := range []string{oldRole, newRole} {
			_, _ = admin.Exec(cleanupCtx, "DROP OWNED BY "+pgx.Identifier{role}.Sanitize())
			_, _ = admin.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize())
		}
		admin.Close()
	})
	createLoginRole(t, ctx, admin, oldRole, oldSecret)
	createLoginRole(t, ctx, admin, newRole, newSecret)
	for _, role := range []string{oldRole, newRole} {
		if _, err := admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatalf("grant schema usage: %v", err)
		}
		if _, err := admin.Exec(ctx, "GRANT SELECT ON credential_rotation_probe TO "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatalf("grant probe read: %v", err)
		}
	}

	oldRuntime := openCredentialRuntime(t, ctx, credentialDSN(t, adminDSN, oldRole, oldSecret))
	assertCredentialProbe(t, ctx, oldRuntime)
	newRuntime := openCredentialRuntime(t, ctx, credentialDSN(t, adminDSN, newRole, newSecret))
	assertCredentialProbe(t, ctx, newRuntime)
	newRuntime.Close()

	invalidDSN := credentialDSN(t, adminDSN, newRole, invalidSecret)
	if runtime, err := Open(ctx, invalidDSN); err == nil {
		_ = runtime.Close()
		t.Fatal("invalid candidate database credential unexpectedly passed preflight")
	} else if strings.Contains(err.Error(), invalidSecret) {
		t.Fatal("database preflight error exposed candidate credential plaintext")
	}
	assertCredentialProbe(t, ctx, oldRuntime)

	rolledRuntime := openCredentialRuntime(t, ctx, credentialDSN(t, adminDSN, newRole, newSecret))
	defer rolledRuntime.Close()
	assertCredentialProbe(t, ctx, rolledRuntime)
	if err := oldRuntime.Close(); err != nil {
		t.Fatalf("close rolled old database pool: %v", err)
	}
	if _, err := admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{oldRole}.Sanitize()+" NOLOGIN"); err != nil {
		t.Fatalf("revoke old database login: %v", err)
	}
	oldDSN := credentialDSN(t, adminDSN, oldRole, oldSecret)
	if runtime, err := Open(ctx, oldDSN); err == nil {
		_ = runtime.Close()
		t.Fatal("revoked old database credential remained usable")
	} else if strings.Contains(err.Error(), oldSecret) {
		t.Fatal("database revocation error exposed old credential plaintext")
	}
	assertCredentialProbe(t, ctx, rolledRuntime)
}

func createLoginRole(t *testing.T, ctx context.Context, admin *pgxpool.Pool, role, secret string) {
	t.Helper()
	var statement string
	if err := admin.QueryRow(ctx, `SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', $1::text, $2::text)`, role, secret).Scan(&statement); err != nil {
		t.Fatalf("build isolated login role statement: %v", err)
	}
	if _, err := admin.Exec(ctx, statement); err != nil {
		t.Fatalf("create isolated login role: %v", err)
	}
}

func credentialDSN(t *testing.T, raw, role, secret string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse isolated database URL: %v", err)
	}
	parsed.User = url.UserPassword(role, secret)
	return parsed.String()
}

func openCredentialRuntime(t *testing.T, ctx context.Context, dsn string) *Runtime {
	t.Helper()
	runtime, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("preflight isolated database credential: %v", err)
	}
	return runtime
}

func assertCredentialProbe(t *testing.T, ctx context.Context, runtime *Runtime) {
	t.Helper()
	var marker string
	if err := runtime.Pool.QueryRow(ctx, `SELECT marker FROM credential_rotation_probe WHERE id = 1`).Scan(&marker); err != nil {
		t.Fatalf("read database credential probe: %v", err)
	}
	if marker != "preserved" {
		t.Fatalf("database credential probe marker = %q", marker)
	}
}
