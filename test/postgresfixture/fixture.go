// Package postgresfixture creates a disposable PostgreSQL database per test.
// It avoids sharing the database named by HOTKEY_TEST_DSN between packages or
// test cases, while preserving that DSN as the administrator connection.
package postgresfixture

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var sequence atomic.Uint64

// New creates a blank disposable database and returns its connection URL.
// Callers own their runtime and schema initialization. HOTKEY_TEST_DSN must
// identify a PostgreSQL role permitted to create and drop databases.
func New(t testing.TB) string {
	t.Helper()
	if err := validateIsolationEnvironment(); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("HOTKEY_TEST_DSN")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	adminConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse HOTKEY_TEST_DSN: %v", err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("open fixture administrator pool: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping fixture administrator pool: %v", err)
	}

	databaseName := databaseName(os.Getpid(), time.Now().UnixNano(), sequence.Add(1))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+databaseName+" TEMPLATE template0"); err != nil {
		admin.Close()
		t.Fatalf("create disposable database: %v", err)
	}
	childDSN, err := withDatabase(dsn, databaseName)
	if err != nil {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+databaseName+" WITH (FORCE)")
		admin.Close()
		t.Fatalf("build disposable database URL: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+databaseName+" WITH (FORCE)")
		admin.Close()
	})
	return childDSN
}

func validateIsolationEnvironment() error {
	testDSN := strings.TrimSpace(os.Getenv("HOTKEY_TEST_DSN"))
	if testDSN == "" {
		return fmt.Errorf("HOTKEY_TEST_DSN is required for PostgreSQL integration tests")
	}
	testPostgres, err := normalizedTarget(testDSN, map[string]bool{"postgres": true, "postgresql": true})
	if err != nil {
		return fmt.Errorf("validate HOTKEY_TEST_DSN: %w", err)
	}
	if runtimeDSN := strings.TrimSpace(os.Getenv("HOTKEY_DATABASE_URL")); runtimeDSN != "" {
		runtimePostgres, err := normalizedTarget(runtimeDSN, map[string]bool{"postgres": true, "postgresql": true})
		if err != nil {
			return fmt.Errorf("validate HOTKEY_DATABASE_URL: %w", err)
		}
		if testPostgres == runtimePostgres {
			return fmt.Errorf("HOTKEY_TEST_DSN must not target HOTKEY_DATABASE_URL")
		}
	}

	testRedisURL := strings.TrimSpace(os.Getenv("HOTKEY_TEST_REDIS_URL"))
	if testRedisURL == "" {
		return fmt.Errorf("HOTKEY_TEST_REDIS_URL is required for integration tests")
	}
	testRedis, err := normalizedTarget(testRedisURL, map[string]bool{"redis": true, "rediss": true})
	if err != nil {
		return fmt.Errorf("validate HOTKEY_TEST_REDIS_URL: %w", err)
	}
	if runtimeRedisURL := strings.TrimSpace(os.Getenv("HOTKEY_REDIS_URL")); runtimeRedisURL != "" {
		runtimeRedis, err := normalizedTarget(runtimeRedisURL, map[string]bool{"redis": true, "rediss": true})
		if err != nil {
			return fmt.Errorf("validate HOTKEY_REDIS_URL: %w", err)
		}
		if testRedis == runtimeRedis {
			return fmt.Errorf("HOTKEY_TEST_REDIS_URL must not target HOTKEY_REDIS_URL")
		}
	}
	return nil
}

func normalizedTarget(raw string, allowedSchemes map[string]bool) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if !allowedSchemes[parsed.Scheme] || parsed.Host == "" {
		return "", fmt.Errorf("unsupported or incomplete target URL")
	}
	parsed.RawQuery = parsed.Query().Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func databaseName(processID int, timestamp int64, ordinal uint64) string {
	return fmt.Sprintf("hotkey_it_%d_%d_%d", processID, timestamp, ordinal)
}

func withDatabase(dsn, databaseName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("HOTKEY_TEST_DSN must use a PostgreSQL URL scheme")
	}
	parsed.Path = "/" + databaseName
	parsed.RawPath = ""
	return parsed.String(), nil
}
