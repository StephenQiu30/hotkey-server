package architecture_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestGreenfieldLayout(t *testing.T) {
	if os.Getenv("HOTKEY_TEST_SUITE_ACTIVE") == "1" {
		return
	}
	root := repositoryRoot(t)
	required := []string{
		"internal/bootstrap",
		"internal/platform",
		"internal/shared",
		"internal/modules",
	}
	for _, relative := range required {
		info, err := os.Stat(filepath.Join(root, relative))
		if err != nil || !info.IsDir() {
			t.Errorf("required directory %s is missing", relative)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "db", "schema.sql")); err != nil || info.IsDir() {
		t.Errorf("required complete schema db/schema.sql is missing")
	}
	if _, err := os.Stat(filepath.Join(root, "db", "schema")); err == nil {
		t.Error("legacy split schema directory db/schema must not exist")
	}
	if _, err := os.Stat(filepath.Join(root, "scripts")); err == nil {
		t.Error("root scripts directory must not exist; test tools belong under test/")
	}

	forbidden := []string{
		"internal/controller",
		"internal/service",
		"internal/repository",
		"internal/model",
		"internal/queue",
		"internal/worker",
		"internal/fxapp",
	}
	for _, relative := range forbidden {
		if _, err := os.Stat(filepath.Join(root, relative)); err == nil {
			t.Errorf("legacy directory %s must not exist", relative)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "test", "_suite")); err != nil || !info.IsDir() {
		t.Error("centralized test suite test/_suite is missing")
	}
	var mixedTests []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "test") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			mixedTests = append(mixedTests, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan test placement: %v", err)
	}
	if len(mixedTests) > 0 {
		t.Errorf("test files must be kept under test/: %s", strings.Join(mixedTests, ", "))
	}
}

func TestLegacyRuntimeDependenciesAreRemoved(t *testing.T) {
	root := repositoryRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range []string{
		"github.com/segmentio/kafka-go",
		"github.com/tmc/langchaingo",
	} {
		if strings.Contains(string(content), dependency) {
			t.Errorf("legacy dependency %s must be removed", dependency)
		}
	}
}
