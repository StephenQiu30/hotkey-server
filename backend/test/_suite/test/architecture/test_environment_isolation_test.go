package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendTestEnvironmentGuardRejectsFormalDatastores(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	script := filepath.Join(repository, "backend", "test", "tools", "validate-test-environment.sh")
	tests := []struct {
		name       string
		dsn        string
		redis      string
		wantOK     bool
		wantOutput string
	}{
		{name: "formal postgres", dsn: "postgres://hotkey:test@127.0.0.1:55432/hotkey?sslmode=disable", redis: "redis://127.0.0.1:56379/15", wantOutput: "disposable PostgreSQL database"},
		{name: "formal redis", dsn: "postgres://hotkey:test@127.0.0.1:55432/hotkey_test?sslmode=disable", redis: "redis://127.0.0.1:56379/0", wantOutput: "disposable Redis database"},
		{name: "isolated stores", dsn: "postgres://hotkey:test@127.0.0.1:55432/hotkey_test?sslmode=disable", redis: "redis://127.0.0.1:56379/15", wantOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("sh", script)
			command.Env = append(os.Environ(), "HOTKEY_TEST_DSN="+test.dsn, "HOTKEY_TEST_REDIS_URL="+test.redis)
			output, err := command.CombinedOutput()
			if test.wantOK && err != nil {
				t.Fatalf("isolated test stores were rejected: %v\n%s", err, output)
			}
			if !test.wantOK && err == nil {
				t.Fatal("unsafe test store was accepted")
			}
			if test.wantOutput != "" && !strings.Contains(string(output), test.wantOutput) {
				t.Fatalf("guard output %q does not contain %q", output, test.wantOutput)
			}
		})
	}

	for _, path := range []string{
		"backend/Makefile",
		"backend/test/tools/verify-database-runtime.sh",
		"backend/test/tools/verify-schema.sh",
		"backend/test/tools/generate-capacity-fixture.sh",
		"backend/test/tools/generate-collection-capacity-fixture.sh",
		"backend/test/tools/generate-search-capacity-fixture.sh",
	} {
		source := readRepositoryFile(t, repository, path)
		if !strings.Contains(source, "validate-test-environment.sh") {
			t.Errorf("destructive or shared test entry %s bypasses the datastore isolation guard", path)
		}
	}
}
