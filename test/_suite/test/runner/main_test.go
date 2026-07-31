package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSuiteEntriesIncludeTestsAndTestdata(t *testing.T) {
	suiteRoot := t.TempDir()
	testFile := writeRunnerFixture(t, suiteRoot, "example/service_test.go", "package example")
	testdataDir := filepath.Join(suiteRoot, "example", "testdata")
	writeRunnerFixture(t, suiteRoot, "example/testdata/input.json", "{}")
	writeRunnerFixture(t, suiteRoot, "example/notes.json", "{}")

	entries, err := suiteEntries(suiteRoot)
	if err != nil {
		t.Fatalf("suiteEntries() error = %v", err)
	}
	want := []string{testFile, testdataDir}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("suiteEntries() = %#v, want %#v", entries, want)
	}
}

func TestMaterializeLinksTestsAndTestdata(t *testing.T) {
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "test", "_suite")
	testFile := writeRunnerFixture(t, suiteRoot, "example/service_test.go", "package example")
	testdataDir := filepath.Join(suiteRoot, "example", "testdata")
	writeRunnerFixture(t, suiteRoot, "example/testdata/input.json", "fixture")

	links, err := materialize(root, suiteRoot, []string{testFile, testdataDir})
	if err != nil {
		t.Fatalf("materialize() error = %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(root, links); err != nil {
			t.Errorf("cleanup() error = %v", err)
		}
	})

	for _, target := range []string{
		filepath.Join(root, "example", "service_test.go"),
		filepath.Join(root, "example", "testdata"),
	} {
		info, err := os.Lstat(target)
		if err != nil {
			t.Fatalf("Lstat(%s): %v", target, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("materialized target %s is not a symbolic link", target)
		}
	}
}

func TestRepositoryErrorRedactsAbsoluteRoot(t *testing.T) {
	root := t.TempDir()
	errorText := repositoryError(root, &os.PathError{
		Op:   "open",
		Path: filepath.Join(root, "internal", "fixture.json"),
		Err:  os.ErrPermission,
	})
	if strings.Contains(errorText, root) {
		t.Fatalf("repositoryError() exposed absolute root: %s", errorText)
	}
	if !strings.Contains(errorText, "internal/fixture.json") {
		t.Fatalf("repositoryError() omitted repository-relative path: %s", errorText)
	}
}

func TestMaterializeConflictCleansEarlierLinks(t *testing.T) {
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "test", "_suite")
	testFile := writeRunnerFixture(t, suiteRoot, "example/service_test.go", "package example")
	testdataDir := filepath.Join(suiteRoot, "example", "testdata")
	writeRunnerFixture(t, suiteRoot, "example/testdata/input.json", "fixture")
	conflictingTarget := filepath.Join(root, "example", "testdata")
	if err := os.MkdirAll(conflictingTarget, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := materialize(root, suiteRoot, []string{testFile, testdataDir}); err == nil {
		t.Fatal("materialize() error = nil, want target conflict")
	}
	if _, err := os.Lstat(filepath.Join(root, "example", "service_test.go")); !os.IsNotExist(err) {
		t.Fatalf("earlier test link remains after conflict: %v", err)
	}
}

func TestRunCleansLinksAfterChildFailure(t *testing.T) {
	if os.Getenv("HOTKEY_RUNNER_FAILURE_HELPER") == "1" {
		os.Exit(7)
	}

	root := t.TempDir()
	suiteRoot := filepath.Join(root, "test", "_suite")
	writeRunnerFixture(t, suiteRoot, "example/service_test.go", "package example")
	writeRunnerFixture(t, suiteRoot, "example/testdata/input.json", "fixture")
	t.Setenv("HOTKEY_RUNNER_FAILURE_HELPER", "1")

	if status := run(root, []string{"-test.run=TestRunCleansLinksAfterChildFailure"}, os.Args[0]); status != 7 {
		t.Fatalf("run() status = %d, want 7", status)
	}
	for _, target := range []string{
		filepath.Join(root, "example", "service_test.go"),
		filepath.Join(root, "example", "testdata"),
	} {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Errorf("materialized target remains after child failure %s: %v", target, err)
		}
	}
}

func TestRunReportsCleanupErrorAfterChildFailure(t *testing.T) {
	if os.Getenv("HOTKEY_RUNNER_CLEANUP_FAILURE_HELPER") == "1" {
		target := filepath.Join(os.Getenv("HOTKEY_RUNNER_TEST_ROOT"), "example", "service_test.go")
		if err := os.Remove(target); err != nil {
			os.Exit(8)
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			os.Exit(8)
		}
		if err := os.WriteFile(filepath.Join(target, "keep"), []byte("fixture"), 0o600); err != nil {
			os.Exit(8)
		}
		os.Exit(7)
	}

	root := t.TempDir()
	suiteRoot := filepath.Join(root, "test", "_suite")
	writeRunnerFixture(t, suiteRoot, "example/service_test.go", "package example")
	t.Setenv("HOTKEY_RUNNER_CLEANUP_FAILURE_HELPER", "1")
	t.Setenv("HOTKEY_RUNNER_TEST_ROOT", root)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = writer
	status := run(root, []string{"-test.run=TestRunReportsCleanupErrorAfterChildFailure"}, os.Args[0])
	os.Stderr = originalStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if status != 7 {
		t.Fatalf("run() status = %d, want original child status 7", status)
	}
	if !strings.Contains(string(output), "test cleanup failed") {
		t.Fatalf("run() did not report cleanup error: %s", output)
	}
	if strings.Contains(string(output), root) {
		t.Fatalf("run() exposed absolute repository root: %s", output)
	}
}

func writeRunnerFixture(t *testing.T, root, relative, contents string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
