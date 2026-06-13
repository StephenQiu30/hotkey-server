package testutil

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// SetupTestDB opens a pgx connection to the test database, verifies
// connectivity, truncates all tables in FK-safe order, and returns
// the ready-to-use *sql.DB.  The test is skipped when no database
// URL is available.
func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	SkipIfNoDB(t)

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("testutil: open db: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("testutil: ping db: %v", err)
	}

	cleanTables(t, db)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// cleanTables truncates every table in FK-dependency order so that
// each test starts from a known-empty state.
func cleanTables(t *testing.T, db *sql.DB) {
	t.Helper()

	tables := []string{
		"email_deliveries",
		"user_notifications",
		"alerts",
		"topic_snapshots",
		"monitor_snapshots",
		"topic_posts",
		"topics",
		"monitor_post_hits",
		"platform_posts",
		"platform_authors",
		"monitor_runs",
		"keyword_monitors",
		"users",
	}

	for _, tbl := range tables {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("testutil: clean table %s: %v", tbl, err)
		}
	}
}
