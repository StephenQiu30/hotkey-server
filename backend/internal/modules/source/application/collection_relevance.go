package application

import (
	"math"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// collectionRelevance is the shared deterministic filter for persisted scans
// and instant search. Connectors may query upstream when supported; this local
// check keeps source-wide feeds such as RSS and Hacker News monitor-scoped.
func collectionRelevance(query, text string) (int, bool) {
	query = normalizedCollectionText(query)
	text = normalizedCollectionText(text)
	if query != "" && containsCollectionQuery(text, query) {
		return 100, true
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0, false
	}
	matched := 0
	for _, term := range terms {
		if containsCollectionQuery(text, term) {
			matched++
		}
	}
	return int(math.Round(float64(matched) / float64(len(terms)) * 80)), false
}

func normalizedCollectionText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(norm.NFKC.String(value)), " "))
}

// containsCollectionQuery keeps substring matching for CJK text while treating
// ASCII letters and digits as words. This prevents short terms such as "AI"
// from matching unrelated words such as "Tailscale" or "available".
func containsCollectionQuery(text, query string) bool {
	if text == "" || query == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(query)
	last, _ := utf8.DecodeLastRuneInString(query)
	for offset := 0; offset <= len(text)-len(query); {
		relative := strings.Index(text[offset:], query)
		if relative < 0 {
			return false
		}
		start := offset + relative
		end := start + len(query)
		leftBounded := true
		if start > 0 && asciiCollectionWordRune(first) {
			before, _ := utf8.DecodeLastRuneInString(text[:start])
			leftBounded = !asciiCollectionWordRune(before)
		}
		rightBounded := true
		if end < len(text) && asciiCollectionWordRune(last) {
			after, _ := utf8.DecodeRuneInString(text[end:])
			rightBounded = !asciiCollectionWordRune(after)
		}
		if leftBounded && rightBounded {
			return true
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		offset = start + size
	}
	return false
}

func asciiCollectionWordRune(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z'
}
