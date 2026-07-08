package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// SetupTestDB opens a GORM connection to the test database, verifies
// connectivity, truncates all tables in FK-safe order, and returns
// the ready-to-use *gorm.DB. The test is skipped when no database URL is available.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	SkipIfNoDB(t)

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("testutil: open db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		t.Fatalf("testutil: ping db: %v", err)
	}

	cleanTables(t, sqlDB)

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("testutil: open gorm: %v", err)
	}

	t.Cleanup(func() {
		if db, err := gdb.DB(); err == nil {
			db.Close()
		}
	})

	return gdb
}

func cleanTables(t *testing.T, db *sql.DB) {
	t.Helper()

	tables := []string{
		"report_exports",
		"knowledge_runs",
		"reports",
		"theme_memberships",
		"topic_annotations",
		"event_annotations",
		"knowledge_writeback_logs",
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, tbl := range tables {
		if _, err := db.ExecContext(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("testutil: clean table %s: %v", tbl, err)
		}
	}
}
