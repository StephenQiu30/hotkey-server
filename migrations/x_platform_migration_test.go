package migrations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXPlatformMigrationContainsRequiredTables(t *testing.T) {
	migrationPath := filepath.Join("000013_x_platform.up.sql")
	sql, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	content := string(sql)

	requirements := []string{
		"CREATE TABLE x_oauth_states",
		"CREATE TABLE x_credentials",
		"source_id text PRIMARY KEY REFERENCES sources",
		"access_token text NOT NULL",
		"refresh_token text NOT NULL DEFAULT ''",
		"code_verifier text NOT NULL",
		"CHECK (type IN ('rss', 'public_page', 'x'))",
		"expires_at timestamptz",
	}

	for _, req := range requirements {
		if !strings.Contains(content, req) {
			t.Errorf("migration missing required content: %s", req)
		}
	}
}

func TestXPlatformDownMigrationRevertsCleanly(t *testing.T) {
	downPath := filepath.Join("000013_x_platform.down.sql")
	sql, err := os.ReadFile(downPath)
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	content := string(sql)

	requirements := []string{
		"DROP TABLE IF EXISTS x_credentials",
		"DROP TABLE IF EXISTS x_oauth_states",
		"CHECK (type IN ('rss', 'public_page'))",
	}

	for _, req := range requirements {
		if !strings.Contains(content, req) {
			t.Errorf("down migration missing required content: %s", req)
		}
	}
}
