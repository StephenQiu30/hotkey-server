package knowledge

import (
	"errors"
	"testing"
)

func TestDetectConflict_OnStaleRevision(t *testing.T) {
	err := DetectConflict(ConflictInput{
		CurrentRevision: "rev-2",
		IncomingRevision: "rev-1",
	})
	if !errors.Is(err, ErrWritebackConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestDetectConflict_OnMatchingRevision(t *testing.T) {
	err := DetectConflict(ConflictInput{
		CurrentRevision: "rev-2",
		IncomingRevision: "rev-2",
	})
	if err != nil {
		t.Fatalf("expected no conflict, got %v", err)
	}
}

func TestDetectConflict_OnEmptyRevision(t *testing.T) {
	err := DetectConflict(ConflictInput{
		CurrentRevision: "",
		IncomingRevision: "",
	})
	if err != nil {
		t.Fatalf("expected no conflict with empty revisions, got %v", err)
	}
}
