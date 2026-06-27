package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	dbassets "github.com/StephenQiu30/hotkey-server/db"
	"github.com/jackc/pgx/v5/pgconn"
)

var dbNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const baselineMigration = "000_schema.sql"

// EnsureReady creates the target database and applies schema.sql when needed.
// Set DB_SKIP_INIT=1 to disable (e.g. production with external migrations).
func EnsureReady(ctx context.Context, databaseURL string) error {
	if os.Getenv("DB_SKIP_INIT") == "1" {
		return nil
	}

	dbName, adminURL, err := parseDatabaseURLs(databaseURL)
	if err != nil {
		return err
	}

	if err := pingOrCreateDatabase(ctx, databaseURL, adminURL, dbName); err != nil {
		return err
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db for init: %w", err)
	}
	defer db.Close()

	ready, err := schemaApplied(ctx, db)
	if err != nil {
		return err
	}
	if !ready {
		if err := applySchema(ctx, db, dbassets.SchemaSQL); err != nil {
			return err
		}
	}

	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}
	if err := markMigrationApplied(ctx, db, baselineMigration); err != nil {
		return err
	}
	return applyPendingMigrations(ctx, db)
}

func parseDatabaseURLs(databaseURL string) (dbName string, adminURL string, err error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", "", fmt.Errorf("parse database url: %w", err)
	}

	dbName = strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", "", errors.New("database url must include a database name")
	}
	if !dbNamePattern.MatchString(dbName) {
		return "", "", fmt.Errorf("invalid database name: %q", dbName)
	}

	admin := *u
	admin.Path = "/postgres"
	return dbName, admin.String(), nil
}

func pingOrCreateDatabase(ctx context.Context, databaseURL, adminURL, dbName string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err == nil {
		return nil
	} else if !isDatabaseMissing(err) {
		return fmt.Errorf("ping db: %w", err)
	}

	adminDB, err := sql.Open("pgx", adminURL)
	if err != nil {
		return fmt.Errorf("open admin db: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping admin db: %w", err)
	}

	var exists bool
	if err := adminDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check database exists: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+dbName); err != nil {
		return fmt.Errorf("create database %q: %w", dbName, err)
	}
	return nil
}

func isDatabaseMissing(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "3D000"
}

func schemaApplied(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'users'
		)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check schema: %w", err)
	}
	return exists, nil
}

func applySchema(ctx context.Context, db *sql.DB, schemaSQL string) error {
	for _, stmt := range splitSQLStatements(schemaSQL) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema statement: %w", err)
		}
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text primary key,
			applied_at timestamptz not null default now()
		)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func markMigrationApplied(ctx context.Context, db *sql.DB, version string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations(version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
		version,
	)
	if err != nil {
		return fmt.Errorf("mark migration %s applied: %w", version, err)
	}
	return nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
		version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return exists, nil
}

func applyPendingMigrations(ctx context.Context, db *sql.DB) error {
	entries, err := dbassets.MigrationFiles()
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version := entry.Name()
		if version == baselineMigration {
			continue
		}
		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := fs.ReadFile(dbassets.MigrationFS, "migrations/"+version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if err := markMigrationApplied(ctx, db, version); err != nil {
			return err
		}
	}
	return nil
}

func splitSQLStatements(schemaSQL string) []string {
	var (
		stmts []string
		buf   strings.Builder
	)

	flush := func() {
		stmt := strings.TrimSpace(buf.String())
		buf.Reset()
		if stmt != "" {
			stmts = append(stmts, stmt)
		}
	}

	for _, line := range strings.Split(schemaSQL, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if strings.HasSuffix(trimmed, ";") {
			flush()
		}
	}

	if buf.Len() > 0 {
		flush()
	}
	return stmts
}
