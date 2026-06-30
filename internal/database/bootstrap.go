package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	dbassets "github.com/StephenQiu30/hotkey-server/db"
	"github.com/jackc/pgx/v5/pgconn"
)

var dbNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// EnsureReady creates the target database and applies schema.sql when needed.
// Set DB_SKIP_INIT=1 to disable automatic local database initialization.
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
	return nil
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
