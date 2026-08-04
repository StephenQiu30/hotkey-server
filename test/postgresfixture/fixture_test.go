package postgresfixture

import "testing"

func TestDatabaseNameSeparatesConcurrentTestProcesses(t *testing.T) {
	first := databaseName(101, 123456789, 1)
	second := databaseName(102, 123456789, 1)
	if first == second {
		t.Fatalf("database names collide across processes: %q", first)
	}
}

func TestValidateIsolationEnvironmentRejectsMissingOrSharedTargets(t *testing.T) {
	t.Setenv("HOTKEY_TEST_DSN", "")
	t.Setenv("HOTKEY_TEST_REDIS_URL", "")
	if err := validateIsolationEnvironment(); err == nil {
		t.Fatal("validateIsolationEnvironment() error = nil, want missing test target rejection")
	}

	t.Setenv("HOTKEY_TEST_DSN", "postgres://hotkey:test@127.0.0.1:5432/hotkey_test?sslmode=disable")
	t.Setenv("HOTKEY_DATABASE_URL", "postgres://hotkey:test@127.0.0.1:5432/hotkey_test?sslmode=disable")
	t.Setenv("HOTKEY_TEST_REDIS_URL", "redis://127.0.0.1:6379/15")
	t.Setenv("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/0")
	if err := validateIsolationEnvironment(); err == nil {
		t.Fatal("validateIsolationEnvironment() error = nil, want shared PostgreSQL target rejection")
	}

	t.Setenv("HOTKEY_DATABASE_URL", "postgres://hotkey:test@127.0.0.1:5432/hotkey_dev?sslmode=disable")
	t.Setenv("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/15")
	if err := validateIsolationEnvironment(); err == nil {
		t.Fatal("validateIsolationEnvironment() error = nil, want shared Redis target rejection")
	}
}

func TestValidateIsolationEnvironmentAcceptsDistinctTargets(t *testing.T) {
	t.Setenv("HOTKEY_TEST_DSN", "postgres://hotkey:test@127.0.0.1:5432/hotkey_test?sslmode=disable")
	t.Setenv("HOTKEY_DATABASE_URL", "postgres://hotkey:test@127.0.0.1:5432/hotkey_dev?sslmode=disable")
	t.Setenv("HOTKEY_TEST_REDIS_URL", "redis://127.0.0.1:6379/15")
	t.Setenv("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/0")
	if err := validateIsolationEnvironment(); err != nil {
		t.Fatalf("validateIsolationEnvironment() error = %v", err)
	}
}
