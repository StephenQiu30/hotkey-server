package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterUsesAtomicStablePath(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root)
	path, err := writer.Write("events", "evt-1", "# event")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "# event" {
		t.Fatalf("written content = %q/%v", contents, err)
	}
	if _, err := writer.Write("events", "../escape", "bad"); err == nil || !strings.Contains(err.Error(), "vault path") {
		t.Fatalf("traversal error = %v", err)
	}
}

func TestWriterAutomaticUpdateAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root)
	if _, err := writer.Write("events", "evt-1", "Human"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAutomatic("events", "evt-1", "Generated v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAutomatic("events", "evt-1", "Generated v2"); err != nil {
		t.Fatal(err)
	}
	content, _, err := writer.Read("events", "evt-1")
	if err != nil || !strings.Contains(string(content), "Human") || !strings.Contains(string(content), "Generated v2") || strings.Contains(string(content), "Generated v1") {
		t.Fatalf("automatic update = %q/%v", content, err)
	}
	escape := filepath.Join(root, "escape")
	if err := os.MkdirAll(escape, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "topics")
	if err := os.Symlink(escape, link); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write("topics", "escaped", "bad"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink escape error = %v", err)
	}
}
