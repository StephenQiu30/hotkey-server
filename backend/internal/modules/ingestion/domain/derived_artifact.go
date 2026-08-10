package domain

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

type DerivedArtifactType string

const (
	DerivedArtifactMarkdown  DerivedArtifactType = "markdown"
	DerivedArtifactPlaintext DerivedArtifactType = "plaintext"
)

func (artifactType DerivedArtifactType) Valid() bool {
	return artifactType == DerivedArtifactMarkdown || artifactType == DerivedArtifactPlaintext
}

func (artifactType DerivedArtifactType) MIMEType() string {
	switch artifactType {
	case DerivedArtifactMarkdown:
		return "text/markdown; charset=utf-8"
	case DerivedArtifactPlaintext:
		return "text/plain; charset=utf-8"
	default:
		return ""
	}
}

type DerivedArtifactLifecycleState string

const (
	DerivedArtifactPending          DerivedArtifactLifecycleState = "derive_pending"
	DerivedArtifactAvailable        DerivedArtifactLifecycleState = "derived_available"
	DerivedArtifactFailed           DerivedArtifactLifecycleState = "derive_failed"
	DerivedArtifactRetentionBlocked DerivedArtifactLifecycleState = "retention_blocked"
	DerivedArtifactQuarantined      DerivedArtifactLifecycleState = "quarantined"
	DerivedArtifactTombstoned       DerivedArtifactLifecycleState = "tombstoned"
)

func (state DerivedArtifactLifecycleState) Valid() bool {
	switch state {
	case DerivedArtifactPending, DerivedArtifactAvailable, DerivedArtifactFailed,
		DerivedArtifactRetentionBlocked, DerivedArtifactQuarantined, DerivedArtifactTombstoned:
		return true
	default:
		return false
	}
}

type DerivedArtifact struct {
	ID                           int64
	SourceConnectionID           int64
	DocumentVersionID            int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	ArtifactType                 DerivedArtifactType
	TransformerProfileSHA256     string
	MIMEType                     string
	SHA256                       string
	SizeBytes                    int64
	LifecycleState               DerivedArtifactLifecycleState
	Active                       bool
	FailureCode                  string
	AvailableAt                  *time.Time
	RetentionUntil               time.Time
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	AnchorMap                    *DocumentAnchorMapIdentity
}

// DocumentAnchorMapIdentity binds a Markdown artifact to the exact NFC
// plaintext identity and immutable offset map used to render stable anchors.
// Individual blocks are persisted separately and are not part of this Entity.
type DocumentAnchorMapIdentity struct {
	NormalizationVersion    string
	AnchorMapProfileVersion string
	PlaintextSHA256         string
	MarkdownSHA256          string
	AnchorMapSHA256         string
}

func (identity DocumentAnchorMapIdentity) Validate() error {
	if strings.TrimSpace(identity.NormalizationVersion) != identity.NormalizationVersion || identity.NormalizationVersion == "" || len(identity.NormalizationVersion) > 64 ||
		strings.TrimSpace(identity.AnchorMapProfileVersion) != identity.AnchorMapProfileVersion || identity.AnchorMapProfileVersion == "" || len(identity.AnchorMapProfileVersion) > 96 ||
		!validLowerDerivedArtifactSHA256(identity.PlaintextSHA256) || !validLowerDerivedArtifactSHA256(identity.MarkdownSHA256) ||
		!validLowerDerivedArtifactSHA256(identity.AnchorMapSHA256) {
		return fmt.Errorf("document anchor map identity is invalid")
	}
	return nil
}

func (artifact DerivedArtifact) Validate() error {
	if artifact.ID <= 0 || artifact.SourceConnectionID <= 0 || artifact.DocumentVersionID <= 0 ||
		artifact.StoreDerivedRightsDecisionID <= 0 || artifact.RetainRightsDecisionID <= 0 ||
		!artifact.ArtifactType.Valid() || artifact.MIMEType != artifact.ArtifactType.MIMEType() ||
		!validLowerDerivedArtifactSHA256(artifact.TransformerProfileSHA256) ||
		!validLowerDerivedArtifactSHA256(artifact.SHA256) || artifact.SizeBytes <= 0 ||
		!artifact.LifecycleState.Valid() || artifact.RetentionUntil.IsZero() || artifact.CreatedAt.IsZero() ||
		artifact.UpdatedAt.IsZero() || artifact.UpdatedAt.Before(artifact.CreatedAt) ||
		!artifact.RetentionUntil.After(artifact.CreatedAt) {
		return fmt.Errorf("derived artifact entity is invalid")
	}
	if artifact.Active && artifact.LifecycleState != DerivedArtifactAvailable {
		return fmt.Errorf("only an available derived artifact can be active")
	}
	if artifact.ArtifactType == DerivedArtifactMarkdown {
		if artifact.AnchorMap == nil || artifact.AnchorMap.Validate() != nil || artifact.AnchorMap.MarkdownSHA256 != artifact.SHA256 {
			return fmt.Errorf("Markdown derived artifact requires an exact anchor map identity")
		}
	} else if artifact.AnchorMap != nil {
		return fmt.Errorf("plaintext derived artifact cannot carry a Markdown anchor map")
	}
	switch artifact.LifecycleState {
	case DerivedArtifactAvailable:
		if artifact.AvailableAt == nil || artifact.AvailableAt.Before(artifact.CreatedAt) || artifact.FailureCode != "" {
			return fmt.Errorf("available derived artifact state is invalid")
		}
	case DerivedArtifactFailed:
		if artifact.AvailableAt != nil || strings.TrimSpace(artifact.FailureCode) == "" || len(artifact.FailureCode) > 64 {
			return fmt.Errorf("failed derived artifact state is invalid")
		}
	default:
		if artifact.AvailableAt != nil || artifact.FailureCode != "" {
			return fmt.Errorf("derived artifact lifecycle metadata is invalid")
		}
	}
	return nil
}

func ValidateDerivedArtifactTransition(from, to DerivedArtifactLifecycleState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("derived artifact lifecycle transition uses an invalid state")
	}
	if from == to {
		return nil
	}
	allowed := map[DerivedArtifactLifecycleState]map[DerivedArtifactLifecycleState]bool{
		DerivedArtifactPending: {
			DerivedArtifactAvailable: true, DerivedArtifactFailed: true, DerivedArtifactQuarantined: true,
		},
		DerivedArtifactFailed: {
			DerivedArtifactPending: true, DerivedArtifactQuarantined: true, DerivedArtifactTombstoned: true,
		},
		DerivedArtifactAvailable: {
			DerivedArtifactRetentionBlocked: true, DerivedArtifactQuarantined: true, DerivedArtifactTombstoned: true,
		},
		DerivedArtifactRetentionBlocked: {DerivedArtifactTombstoned: true},
		DerivedArtifactQuarantined:      {DerivedArtifactTombstoned: true},
	}
	if !allowed[from][to] {
		return fmt.Errorf("invalid derived artifact lifecycle transition %s -> %s", from, to)
	}
	return nil
}

func validLowerDerivedArtifactSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}
