// Package aggregator implements cross-platform hot event matching.
//
// It takes X-clustered Topics and platform trending items and merges them
// into unified HotEvents using cosine similarity and keyword overlap.
package aggregator

import (
	"math"
	"strings"
	"time"
	"unicode"
)

const cjkRuneThreshold = 0.5 // if >=50% of runes in a token are CJK, do char-level n-grams

// EventMatcher determines whether two text sources refer to the same event.
type EventMatcher struct {
	CosineThreshold      float64
	KeywordOverlapThreshold float64
	TimeWindow           time.Duration
}

// DefaultMatcher returns an EventMatcher with sensible defaults.
func DefaultMatcher() *EventMatcher {
	return &EventMatcher{
		CosineThreshold:        0.5,
		KeywordOverlapThreshold: 0.3,
		TimeWindow:              24 * time.Hour,
	}
}

// MatchResult describes the outcome of comparing two sources.
type MatchResult struct {
	Score           float64
	CosineScore     float64
	KeywordOverlap  float64
	IsMatch         bool
}

// Match determines if two text sources refer to the same event.
func (m *EventMatcher) Match(a, b string, tA, tB time.Time) *MatchResult {
	// 1. Time window check
	if !tA.IsZero() && !tB.IsZero() {
		diff := tA.Sub(tB)
		if diff < 0 {
			diff = -diff
		}
		if diff > m.TimeWindow {
			return &MatchResult{IsMatch: false}
		}
	}

	// 2. Extract tokens
	tokensA := extractTokens(a)
	tokensB := extractTokens(b)

	if len(tokensA) == 0 || len(tokensB) == 0 {
		return &MatchResult{IsMatch: false}
	}

	// 3. Keyword overlap
	overlap := keywordOverlap(tokensA, tokensB)
	keywordScore := overlap
	if len(tokensA) > 0 && len(tokensB) > 0 {
		keywordScore = 2.0 * float64(overlap) / float64(len(tokensA)+len(tokensB))
	}

	// 4. Cosine similarity
	cosineScore := cosineSimilarity(tokensA, tokensB)

	// 5. Weighted combination
	score := 0.6*cosineScore + 0.4*keywordScore

	return &MatchResult{
		Score:          score,
		CosineScore:    cosineScore,
		KeywordOverlap: keywordScore,
		IsMatch:        score >= m.CosineThreshold,
	}
}

// extractTokens splits text into normalized tokens (lowercase, trimmed, no punctuation).
// For CJK-heavy tokens, also generates character bigrams to improve matching.
func extractTokens(text string) []string {
	text = strings.ToLower(text)
	text = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return -1
		}
		return r
	}, text)

	raw := strings.Fields(text)
	tokens := make([]string, 0, len(raw)*2)
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if len(t) <= 1 {
			continue
		}
		tokens = append(tokens, t)

		// For CJK-heavy tokens, also add character bigrams
		if isCJKToken(t) {
			runes := []rune(t)
			for i := 0; i < len(runes)-1; i++ {
				bigram := string(runes[i : i+2])
				if len(bigram) > 0 {
					tokens = append(tokens, bigram)
				}
			}
		}
	}
	return tokens
}

// isCJKToken returns true if at least cjkRuneThreshold of the token's runes are CJK.
func isCJKToken(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	var cjkCount int
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			cjkCount++
		}
	}
	return float64(cjkCount)/float64(len(runes)) >= cjkRuneThreshold
}

// keywordOverlap counts how many tokens appear in both sets.
func keywordOverlap(a, b []string) float64 {
	set := make(map[string]struct{}, len(a))
	for _, t := range a {
		set[t] = struct{}{}
	}
	var count int
	for _, t := range b {
		if _, ok := set[t]; ok {
			count++
		}
	}
	return float64(count)
}

// cosineSimilarity computes the cosine similarity between two token sets using TF vectors.
func cosineSimilarity(a, b []string) float64 {
	tfA := termFreq(a)
	tfB := termFreq(b)

	// Collect all unique terms
	allTerms := make(map[string]struct{})
	for t := range tfA {
		allTerms[t] = struct{}{}
	}
	for t := range tfB {
		allTerms[t] = struct{}{}
	}

	var dot, magA, magB float64
	for t := range allTerms {
		fA := tfA[t]
		fB := tfB[t]
		dot += fA * fB
		magA += fA * fA
		magB += fB * fB
	}

	if magA == 0 || magB == 0 {
		return 0
	}

	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func termFreq(tokens []string) map[string]float64 {
	freq := make(map[string]float64, len(tokens))
	for _, t := range tokens {
		freq[t]++
	}
	// Normalize
	total := float64(len(tokens))
	for t, c := range freq {
		freq[t] = c / total
	}
	return freq
}
