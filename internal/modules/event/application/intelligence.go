package application

import (
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type EvidenceValidator struct{}

func NewEvidenceValidator() *EvidenceValidator { return &EvidenceValidator{} }

func (validator *EvidenceValidator) ValidateSummary(summary domain.EventSummary, activeContentIDs map[int64]bool) error {
	if validator == nil {
		return fmt.Errorf("evidence validator is required")
	}
	if err := summary.Validate(activeContentIDs); err != nil {
		return err
	}
	return nil
}

// DegradedSummary creates a fact-only fallback when no provider is available.
// It never fabricates a conclusion and still requires a representative active
// Content reference.
func DegradedSummary(titleZH, titleEN string, representative domain.EvidenceRef) (domain.EventSummary, error) {
	if strings.TrimSpace(titleZH) == "" || representative.ContentID <= 0 {
		return domain.EventSummary{}, fmt.Errorf("title and representative evidence are required")
	}
	if err := representative.Validate(); err != nil {
		return domain.EventSummary{}, err
	}
	return domain.EventSummary{Version: "fallback-v1", TitleZH: titleZH, TitleEN: titleEN, Degraded: true, Sentences: []domain.EvidenceSentence{{Text: titleZH, Evidence: []domain.EvidenceRef{representative}}}}, nil
}
