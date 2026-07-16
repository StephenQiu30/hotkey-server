package domain

import "testing"

func TestContentStatusAndDedupeDecisionValidation(t *testing.T) {
	t.Parallel()

	for _, status := range []ContentStatus{ContentStatusActive, ContentStatusInvalid, ContentStatusDuplicate, ContentStatusDeleted, ContentStatusExpired} {
		if !status.Valid() {
			t.Errorf("status %q is not valid", status)
		}
	}
	duplicateID := int64(7)
	if err := (DedupeDecision{Status: ContentStatusDuplicate, DuplicateOfID: &duplicateID, Reason: DedupeReasonExactURL, Version: DedupeVersionExactURL}).Validate(); err != nil {
		t.Fatalf("valid duplicate decision error = %v", err)
	}
	if err := (DedupeDecision{Status: ContentStatusDuplicate}).Validate(); err == nil {
		t.Fatal("incomplete duplicate decision error = nil")
	}
	if err := (DedupeDecision{Status: ContentStatusActive, DuplicateOfID: &duplicateID}).Validate(); err == nil {
		t.Fatal("active decision with target error = nil")
	}
}
