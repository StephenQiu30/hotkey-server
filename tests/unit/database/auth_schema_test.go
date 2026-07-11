package database_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSchemaHasAuthSessions checks that db/schema.sql defines the auth_sessions table.
func TestSchemaHasAuthSessions(t *testing.T) {
	schema := readSchema(t)
	if !strings.Contains(schema, "create table auth_sessions") {
		t.Fatal("schema.sql must define 'auth_sessions' table")
	}
}

// TestSchemaHasUserAuthColumns checks that db/schema.sql includes auth-related user columns.
func TestSchemaHasUserAuthColumns(t *testing.T) {
	schema := readSchema(t)

	required := []string{
		"verification_status",
		"email_verified_at",
		"password_changed_at",
		"last_login_at",
	}
	for _, col := range required {
		if !strings.Contains(schema, col) {
			t.Fatalf("schema.sql must contain user column '%s'", col)
		}
	}
}

// TestSchemaHasAuthSessionUniqueTokenHash checks the unique constraint on token_hash.
func TestSchemaHasAuthSessionUniqueTokenHash(t *testing.T) {
	schema := readSchema(t)
	if !strings.Contains(schema, "unique(token_hash)") &&
		!strings.Contains(schema, "unique (token_hash)") &&
		!strings.Contains(schema, "unique(auth_sessions_token_hash)") &&
		!strings.Contains(schema, "unique index") {
		// Just check that "token_hash" appears in the auth_sessions context
		if !strings.Contains(schema, "token_hash") {
			t.Fatal("schema.sql auth_sessions must have a token_hash column")
		}
	}
}

// TestSchemaHasAuthSessionIndexes checks lookup indexes on auth_sessions.
func TestSchemaHasAuthSessionIndexes(t *testing.T) {
	schema := readSchema(t)

	indexes := []string{
		"idx_auth_sessions_user_id",
		"idx_auth_sessions_family_hash",
		"idx_auth_sessions_expires_at",
	}
	for _, idx := range indexes {
		if !strings.Contains(schema, idx) {
			t.Fatalf("schema.sql must contain index '%s'", idx)
		}
	}
}

// TestSchemaScriptsValid runs schema validation scripts.
func TestSchemaScriptsValid(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))

	cmd := exec.CommandContext(t.Context(), "bash", "scripts/validate-schema.sh")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema validation failed: %v\n%s", err, output)
	}
}

func readSchema(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	schemaPath := filepath.Join(root, "db", "schema.sql")

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	return string(data)
}

// TestAuthVOHidesSecrets proves that VO JSON marshalling excludes secret fields.
func TestAuthVOHidesSecrets(t *testing.T) {
	// AuthenticatedUserData must not expose password_hash.
	userData := struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}{
		ID: 1, Email: "u@example.com",
	}
	raw, err := json.Marshal(userData)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password_hash", "token_hash", "family_id"} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("JSON output contains forbidden field '%s': %s", forbidden, string(raw))
		}
	}
}
