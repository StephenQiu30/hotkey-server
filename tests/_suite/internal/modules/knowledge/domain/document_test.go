package domain

import "testing"

func TestStablePathRejectsTraversal(t *testing.T) {
	if _, err := StablePath("/tmp/vault", "events", "../escape"); err == nil {
		t.Fatal("path traversal accepted")
	}
	path, err := StablePath("/tmp/vault", "events", "evt-1")
	if err != nil || path != "/tmp/vault/events/evt-1.md" {
		t.Fatalf("path = %q/%v", path, err)
	}
}
