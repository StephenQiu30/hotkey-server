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
	root := repositoryRoot(t)
	required := []string{
		"internal/bootstrap",
		"internal/platform",
		"internal/shared",
		"internal/modules",
		"db/migrations",
	}
	for _, relative := range required {
		info, err := os.Stat(filepath.Join(root, relative))
		if err != nil || !info.IsDir() {
			t.Errorf("required directory %s is missing", relative)
		}
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
		"github.com/redis/go-redis",
	} {
		if strings.Contains(string(content), dependency) {
			t.Errorf("legacy dependency %s must be removed", dependency)
		}
	}
}
