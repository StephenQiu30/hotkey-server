package domain

import (
	"errors"
	"strings"
)

type EvidenceSummarySentence struct {
	Text           string
	EditorialNote  bool
	EvidenceIDs    []int64
	DecisionOrigin string
	ModelRunID     *int64
	ActorUserID    *int64
}

func ValidateEvidenceSummarySentences(sentences []EvidenceSummarySentence) error {
	if len(sentences) == 0 || len(sentences) > 32 {
		return errors.New("summary requires one to thirty-two sentences")
	}
	for _, sentence := range sentences {
		if strings.TrimSpace(sentence.Text) == "" || len(sentence.Text) > 8000 {
			return errors.New("summary sentence text is invalid")
		}
		if sentence.EditorialNote && len(sentence.EvidenceIDs) != 0 || !sentence.EditorialNote && len(sentence.EvidenceIDs) == 0 {
			return errors.New("report sentences require evidence while editorial notes cannot cite evidence")
		}
		switch sentence.DecisionOrigin {
		case "automatic":
			if sentence.EditorialNote || sentence.ModelRunID == nil || *sentence.ModelRunID <= 0 || sentence.ActorUserID != nil {
				return errors.New("automatic summary sentence provenance is invalid")
			}
		case "manual":
			if sentence.ModelRunID != nil || sentence.ActorUserID == nil || *sentence.ActorUserID <= 0 {
				return errors.New("manual summary sentence provenance is invalid")
			}
		default:
			return errors.New("summary sentence origin is invalid")
		}
		seen := map[int64]struct{}{}
		for _, evidenceID := range sentence.EvidenceIDs {
			if evidenceID <= 0 {
				return errors.New("summary citation is invalid")
			}
			if _, found := seen[evidenceID]; found {
				return errors.New("summary citation is duplicated")
			}
			seen[evidenceID] = struct{}{}
		}
	}
	return nil
}
