package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAtomic_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "---\ntitle: test\n---\n"

	if err := WriteAtomic(path, content); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", string(got), content)
	}
}

func TestWriteAtomic_NoTmpResidual(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	if err := WriteAtomic(path, "content"); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("residual .tmp file found: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(entries), entries)
	}
}

func TestWriteAtomic_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "test.md")

	if err := WriteAtomic(path, "content"); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "content" {
		t.Fatalf("content = %q, want %q", string(got), "content")
	}
}

func TestWriteAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	if err := WriteAtomic(path, "old"); err != nil {
		t.Fatalf("WriteAtomic old: %v", err)
	}
	if err := WriteAtomic(path, "new"); err != nil {
		t.Fatalf("WriteAtomic new: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", string(got), "new")
	}
}

func TestWriteAtomic_InvalidPath(t *testing.T) {
	err := WriteAtomic("/nonexistent/root/test.md", "content")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}
