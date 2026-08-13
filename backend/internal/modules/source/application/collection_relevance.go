package application

import (
	"math"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// collectionRelevance is the shared deterministic filter for persisted scans
// and instant search. Connectors may query upstream when supported; this local
// check keeps source-wide feeds such as RSS and Hacker News monitor-scoped.
func collectionRelevance(query, text string) (int, bool) {
	query = normalizedCollectionText(query)
	text = normalizedCollectionText(text)
	if query != "" && strings.Contains(text, query) {
		return 100, true
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0, false
	}
	matched := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			matched++
		}
	}
	return int(math.Round(float64(matched) / float64(len(terms)) * 80)), false
}

func normalizedCollectionText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(norm.NFKC.String(value)), " "))
}
