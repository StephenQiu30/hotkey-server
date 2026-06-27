package database_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestArchitectureBoundariesScript(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	cmd := exec.CommandContext(t.Context(), "bash", "scripts/validate-architecture-boundaries.sh")
	cmd.Dir = root

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("architecture boundary validation failed: %v\n%s", err, output)
	}
}
