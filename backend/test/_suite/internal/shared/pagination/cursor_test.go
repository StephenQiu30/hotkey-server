package pagination

import (
	"errors"
	"testing"
)

func TestCursorBindsVersionSortAndFilter(t *testing.T) {
	encoded, err := Encode("id", false, "monitor=1", 42)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	cursor, err := Decode(encoded, "id", false, "monitor=1")
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if cursor.ID != 42 {
		t.Fatalf("cursor ID = %d, want 42", cursor.ID)
	}
	if _, err := Decode(encoded, "id", false, "monitor=2"); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("filter mismatch error = %v, want stale cursor", err)
	}
	if _, err := Decode(encoded, "id", true, "monitor=1"); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("direction mismatch error = %v, want stale cursor", err)
	}
	if _, err := Decode("not-a-cursor", "id", false, "monitor=1"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want invalid cursor", err)
	}
}
