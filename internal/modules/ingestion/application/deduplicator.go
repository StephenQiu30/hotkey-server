package application

import (
	"slices"
	"strings"
	"time"
	"unicode"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
)

const nearTextWindow = 24 * time.Hour

// DecideDuplicate applies PLAN-007's fixed, deterministic duplicate rules.
// Exact URL and content hash matches can span sources. Near text is deliberately
// restricted to one source connection so independent reporting stays active.
func DecideDuplicate(content ingestiondomain.NormalizedContent, candidates []ingestiondomain.ContentCandidate) (ingestiondomain.DedupeDecision, error) {
	if err := content.Validate(); err != nil {
		return ingestiondomain.DedupeDecision{}, err
	}
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return ingestiondomain.DedupeDecision{}, err
		}
	}
	if candidate, ok := preferredMatchingCandidate(candidates, func(candidate ingestiondomain.ContentCandidate) bool {
		return candidate.CanonicalURL != "" && candidate.CanonicalURL == content.CanonicalURL
	}); ok {
		return duplicateDecision(candidate.ID, ingestiondomain.DedupeReasonExactURL, ingestiondomain.DedupeVersionExactURL), nil
	}
	if candidate, ok := preferredMatchingCandidate(candidates, func(candidate ingestiondomain.ContentCandidate) bool {
		return candidate.DedupeKey != "" && candidate.DedupeKey == content.ContentHash
	}); ok {
		return duplicateDecision(candidate.ID, ingestiondomain.DedupeReasonExactHash, ingestiondomain.DedupeVersionExactHash), nil
	}
	titleTokens := tokenize(content.Title)
	bodyTokens := tokenize(content.Body)
	if len(titleTokens) == 0 || len(bodyTokens) == 0 {
		return ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive}, nil
	}
	if candidate, ok := preferredMatchingCandidate(candidates, func(candidate ingestiondomain.ContentCandidate) bool {
		if candidate.SourceConnectionID != content.SourceConnectionID || absDuration(candidate.PublishedAt.Sub(content.PublishedAt)) > nearTextWindow {
			return false
		}
		candidateTitleTokens := tokenize(strings.Join(candidate.TitleTokens, " "))
		candidateBodyTokens := tokenize(strings.Join(candidate.BodyTokens, " "))
		return len(candidateBodyTokens) > 0 && slices.Equal(titleTokens, candidateTitleTokens) && jaccardSimilarity(bodyTokens, candidateBodyTokens) >= 0.98
	}); ok {
		return duplicateDecision(candidate.ID, ingestiondomain.DedupeReasonNearText, ingestiondomain.DedupeVersionNearText), nil
	}
	return ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive}, nil
}

func duplicateDecision(id int64, reason, version string) ingestiondomain.DedupeDecision {
	return ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusDuplicate, DuplicateOfID: &id, Reason: reason, Version: version}
}

func preferredMatchingCandidate(candidates []ingestiondomain.ContentCandidate, matches func(ingestiondomain.ContentCandidate) bool) (ingestiondomain.ContentCandidate, bool) {
	var selected ingestiondomain.ContentCandidate
	found := false
	for _, candidate := range candidates {
		if !matches(candidate) || (found && !candidatePreferred(candidate, selected)) {
			continue
		}
		selected = candidate
		found = true
	}
	return selected, found
}

func candidatePreferred(candidate, selected ingestiondomain.ContentCandidate) bool {
	if candidate.Completeness != selected.Completeness {
		return candidate.Completeness > selected.Completeness
	}
	if !candidate.PublishedAt.Equal(selected.PublishedAt) {
		return candidate.PublishedAt.Before(selected.PublishedAt)
	}
	if candidate.SourceExternalIDStable != selected.SourceExternalIDStable {
		return candidate.SourceExternalIDStable
	}
	return candidate.ID < selected.ID
}

func tokenize(value string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(current.String()))
		current.Reset()
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			current.WriteRune(character)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func jaccardSimilarity(left, right []string) float64 {
	leftSet := make(map[string]struct{}, len(left))
	for _, token := range left {
		leftSet[token] = struct{}{}
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, token := range right {
		rightSet[token] = struct{}{}
	}
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for token := range leftSet {
		if _, found := rightSet[token]; found {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	return float64(intersection) / float64(union)
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
