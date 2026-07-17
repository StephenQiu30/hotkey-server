package vault

import (
	"os"
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
