package obsidian_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func TestWriteAtomicNoOverwriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "HotKey", "digests", "daily", "2026-07-08", "ai.md")

	result, err := service.WriteAtomicNoOverwrite(path, []byte("# Daily"))
	if err != nil {
		t.Fatalf("WriteAtomicNoOverwrite returned error: %v", err)
	}
	if result.Status != dto.WriteStatusPublished || result.Skipped {
		t.Fatalf("result = %+v, want published not skipped", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "# Daily" {
		t.Fatalf("file content = %q", string(got))
	}
}

func TestWriteAtomicNoOverwriteSkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("edited in obsidian"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := service.WriteAtomicNoOverwrite(path, []byte("new generated content"))
	if err != nil {
		t.Fatalf("WriteAtomicNoOverwrite returned error: %v", err)
	}
	if result.Status != dto.WriteStatusSkipped || !result.Skipped {
		t.Fatalf("result = %+v, want skipped", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "edited in obsidian" {
		t.Fatalf("existing file was overwritten: %q", string(got))
	}
}
