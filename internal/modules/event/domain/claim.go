package domain

import (
	"fmt"
	"strings"
)

type ClaimStatus string

const (
	ClaimUnverified   ClaimStatus = "unverified"
	ClaimSingleSource ClaimStatus = "single_source"
	ClaimCorroborated ClaimStatus = "corroborated"
	ClaimDisputed     ClaimStatus = "disputed"
	ClaimRetracted    ClaimStatus = "retracted"
)

type EvidenceRef struct {
	ContentID int64
	Locator   string
	Excerpt   string
}

func (evidence EvidenceRef) Validate() error {
	if evidence.ContentID <= 0 || strings.TrimSpace(evidence.Locator) == "" || len(evidence.Locator) > 512 || len(evidence.Excerpt) > 500 {
		return fmt.Errorf("invalid evidence reference")
	}
	return nil
}

type ClaimEvidence struct {
	ID, ClaimID, Version int64
	EvidenceRef
	Stance     string
	Confidence float64
}

func (evidence ClaimEvidence) Validate() error {
	if err := evidence.EvidenceRef.Validate(); err != nil {
		return err
	}
	if evidence.Confidence < 0 || evidence.Confidence > 100 {
		return fmt.Errorf("invalid claim evidence confidence")
	}
	switch evidence.Stance {
	case "supports", "contradicts", "mentions":
		return nil
	default:
		return fmt.Errorf("invalid claim evidence stance")
	}
}

type Claim struct {
	ID, Version, EventID       int64
	NormalizedClaim, ClaimHash string
	Status                     ClaimStatus
	Confidence                 float64
	Evidence                   []ClaimEvidence
	ManualLocked               bool
}

func (claim Claim) Validate() error {
	if claim.ID <= 0 || claim.Version <= 0 || claim.EventID <= 0 || strings.TrimSpace(claim.NormalizedClaim) == "" || len(claim.ClaimHash) != 64 || claim.Confidence < 0 || claim.Confidence > 100 {
		return fmt.Errorf("invalid claim")
	}
	switch claim.Status {
	case ClaimUnverified, ClaimSingleSource, ClaimCorroborated, ClaimDisputed, ClaimRetracted:
	default:
		return fmt.Errorf("invalid claim status")
	}
	for _, evidence := range claim.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type EvidenceSentence struct {
	Text     string
	Evidence []EvidenceRef
}

type EventSummary struct {
	Version          string
	TitleZH, TitleEN string
	Sentences        []EvidenceSentence
	Degraded         bool
}

func (summary EventSummary) Validate(activeContentIDs map[int64]bool) error {
	if strings.TrimSpace(summary.Version) == "" || len(summary.Version) > 64 || strings.TrimSpace(summary.TitleZH) == "" || len(summary.TitleZH) > 500 || len(summary.TitleEN) > 500 || len(summary.Sentences) > 32 {
		return fmt.Errorf("invalid event summary")
	}
	for _, sentence := range summary.Sentences {
		if strings.TrimSpace(sentence.Text) == "" || len(sentence.Text) > 2000 || len(sentence.Evidence) == 0 {
			return fmt.Errorf("summary sentence requires evidence")
		}
		for _, evidence := range sentence.Evidence {
			if err := evidence.Validate(); err != nil || !activeContentIDs[evidence.ContentID] {
				return fmt.Errorf("summary contains inactive evidence")
			}
		}
	}
	return nil
}
