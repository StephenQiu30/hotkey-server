package domain

import (
	"strings"
	"testing"
	"time"
)

func TestDerivedArtifactEntityValidatesLifecycleMetadata(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, time.August, 9, 9, 0, 0, 0, time.UTC)
	availableAt := createdAt.Add(time.Minute)
	artifact := DerivedArtifact{
		ID: 1, SourceConnectionID: 2, DocumentVersionID: 3,
		StoreDerivedRightsDecisionID: 4, RetainRightsDecisionID: 5,
		ArtifactType: DerivedArtifactMarkdown, TransformerProfileSHA256: strings.Repeat("a", 64),
		MIMEType: "text/markdown; charset=utf-8", SHA256: strings.Repeat("b", 64), SizeBytes: 12,
		LifecycleState: DerivedArtifactAvailable, Active: true, AvailableAt: &availableAt,
		RetentionUntil: createdAt.Add(30 * 24 * time.Hour), CreatedAt: createdAt, UpdatedAt: availableAt,
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := artifact
	invalid.LifecycleState = DerivedArtifactFailed
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted active failed artifact with available metadata")
	}
	invalid = artifact
	invalid.TransformerProfileSHA256 = strings.Repeat("A", 64)
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted uppercase transformer profile digest")
	}
}

func TestDerivedArtifactTransitionRejectsRecoveryFromQuarantine(t *testing.T) {
	t.Parallel()
	if err := ValidateDerivedArtifactTransition(DerivedArtifactFailed, DerivedArtifactPending); err != nil {
		t.Fatalf("failed -> pending error = %v", err)
	}
	if err := ValidateDerivedArtifactTransition(DerivedArtifactQuarantined, DerivedArtifactPending); err == nil {
		t.Fatal("quarantined -> pending was accepted")
	}
	if err := ValidateDerivedArtifactTransition(DerivedArtifactAvailable, DerivedArtifactFailed); err == nil {
		t.Fatal("available -> failed was accepted")
	}
}
