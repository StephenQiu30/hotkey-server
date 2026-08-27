package application

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

type structuredEvidenceReference struct {
	ContentID int64  `json:"content_id"`
	Locator   string `json:"locator"`
	Excerpt   string `json:"excerpt"`
}

// validateStructuredOutputPolicy applies value-level rules that JSON Schema
// cannot express. It is executed before persistence and again for every
// candidate reusable result.
func validateStructuredOutputPolicy(taskType domain.TaskType, schemaVersion string, input, output json.RawMessage) error {
	switch {
	case schemaVersion == "v1" && (taskType == domain.TaskTypeEventSummary || taskType == domain.TaskTypeEntityClaimExtraction):
		return validateEventEvidenceWhitelist(taskType, input, output)
	case schemaVersion == "v2" && taskType == domain.TaskTypeEntityClaimExtraction:
		return validateExactQuoteWhitelist(input, output)
	default:
		return nil
	}
}

func validateEventEvidenceWhitelist(taskType domain.TaskType, input, output json.RawMessage) error {
	var source struct {
		Evidence []structuredEvidenceReference `json:"evidence"`
	}
	if json.Unmarshal(input, &source) != nil || len(source.Evidence) == 0 {
		return domain.NewError(domain.CodeAIOutputInvalid)
	}
	allowed := make(map[string]string, len(source.Evidence))
	for _, reference := range source.Evidence {
		allowed[evidenceReferenceKey(reference)] = reference.Excerpt
	}
	var result struct {
		Sentences []struct {
			Evidence []structuredEvidenceReference `json:"evidence"`
		} `json:"sentences"`
		Claims []struct {
			Evidence []structuredEvidenceReference `json:"evidence"`
		} `json:"claims"`
	}
	if json.Unmarshal(output, &result) != nil {
		return domain.NewError(domain.CodeAIOutputInvalid)
	}
	collections := make([][]structuredEvidenceReference, 0, len(result.Sentences)+len(result.Claims))
	if taskType == domain.TaskTypeEventSummary {
		for _, sentence := range result.Sentences {
			collections = append(collections, sentence.Evidence)
		}
	} else {
		for _, claim := range result.Claims {
			collections = append(collections, claim.Evidence)
		}
	}
	for _, references := range collections {
		for _, reference := range references {
			excerpt, exists := allowed[evidenceReferenceKey(reference)]
			if !exists || reference.Excerpt != "" && reference.Excerpt != excerpt {
				return domain.NewError(domain.CodeAIOutputInvalid)
			}
		}
	}
	return nil
}

func evidenceReferenceKey(reference structuredEvidenceReference) string {
	return strconv.FormatInt(reference.ContentID, 10) + "\x00" + reference.Locator
}

func validateExactQuoteWhitelist(input, output json.RawMessage) error {
	var source struct {
		Body string `json:"body"`
	}
	var result struct {
		Claims []struct {
			ExactQuote string `json:"exact_quote"`
		} `json:"claims"`
	}
	if json.Unmarshal(input, &source) != nil || json.Unmarshal(output, &result) != nil || source.Body == "" {
		return domain.NewError(domain.CodeAIOutputInvalid)
	}
	for _, claim := range result.Claims {
		if claim.ExactQuote == "" || !strings.Contains(source.Body, claim.ExactQuote) {
			return domain.NewError(domain.CodeAIOutputInvalid)
		}
	}
	return nil
}
