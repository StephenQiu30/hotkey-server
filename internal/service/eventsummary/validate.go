package eventsummary

import "fmt"

const minSourceCountForFullConfidence = 3

// ValidateSummary checks that an EventSummary has all required fields and valid values.
func ValidateSummary(s EventSummary) error {
	if s.Title == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if s.Summary == "" {
		return fmt.Errorf("%w: summary is required", ErrInvalidInput)
	}
	if s.Confidence < 0 || s.Confidence > 1.0 {
		return fmt.Errorf("%w: confidence must be in [0, 1.0]", ErrInvalidInput)
	}
	return nil
}

// IsLowEvidence returns true when the source count is below the minimum threshold.
func IsLowEvidence(sourceCount int) bool {
	return sourceCount < minSourceCountForFullConfidence
}

// MaxConfidence returns the maximum allowed confidence based on source count.
// Single source: 0.3, two sources: 0.5, three or more: 1.0.
func MaxConfidence(sourceCount int) float64 {
	switch {
	case sourceCount <= 1:
		return 0.3
	case sourceCount == 2:
		return 0.5
	default:
		return 1.0
	}
}
