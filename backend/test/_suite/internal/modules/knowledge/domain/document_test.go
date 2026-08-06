package domain

import (
	"strings"
	"testing"
)

func TestStablePathRejectsTraversal(t *testing.T) {
	if _, err := StablePath("/tmp/vault", "events", "../escape"); err == nil {
		t.Fatal("path traversal accepted")
	}
	path, err := StablePath("/tmp/vault", "events", "evt-1")
	if err != nil || path != "/tmp/vault/events/evt-1.md" {
		t.Fatalf("path = %q/%v", path, err)
	}
}

func TestMergeAutomaticRegionPreservesHumanNotes(t *testing.T) {
	existing := "# Note\n\nHuman note.\n\n" + AutomaticRegionBegin + "\nold\n" + AutomaticRegionEnd + "\n"
	merged, err := MergeAutomaticRegion(existing, "new generated facts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(merged, "Human note.") || strings.Contains(merged, "old") || !strings.Contains(merged, "new generated facts") {
		t.Fatalf("merged document lost manual/automatic content: %q", merged)
	}
}
