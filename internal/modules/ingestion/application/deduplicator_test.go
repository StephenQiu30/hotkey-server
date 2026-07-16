package application

import (
	"fmt"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
)

func TestDecideDuplicatePrioritizesExactURLAndHashAcrossSources(t *testing.T) {
	t.Parallel()

	content := mustNormalize(t, 31, "https://example.test/news?b=2", "A distinct title", "A distinct body")
	urlCandidate := candidateFor(t, 7, 99, "https://example.test/news?b=2", strings.Repeat("e", 64), "other title", "other body", content.PublishedAt)
	hashCandidate := candidateFor(t, 4, 100, "https://other.example.test/news", content.ContentHash, "other title", "other body", content.PublishedAt)

	decision, err := DecideDuplicate(content, []ingestiondomain.ContentCandidate{hashCandidate, urlCandidate})
	if err != nil {
		t.Fatalf("DecideDuplicate() error = %v", err)
	}
	if decision.Status != ingestiondomain.ContentStatusDuplicate || decision.DuplicateOfID == nil || *decision.DuplicateOfID != 7 || decision.Reason != ingestiondomain.DedupeReasonExactURL || decision.Version != ingestiondomain.DedupeVersionExactURL {
		t.Fatalf("decision = %#v, want URL duplicate before hash with lowest stable target", decision)
	}

	decision, err = DecideDuplicate(content, []ingestiondomain.ContentCandidate{hashCandidate})
	if err != nil {
		t.Fatalf("DecideDuplicate(hash) error = %v", err)
	}
	if decision.Status != ingestiondomain.ContentStatusDuplicate || decision.DuplicateOfID == nil || *decision.DuplicateOfID != 4 || decision.Reason != ingestiondomain.DedupeReasonExactHash || decision.Version != ingestiondomain.DedupeVersionExactHash {
		t.Fatalf("hash decision = %#v", decision)
	}
}

func TestDecideDuplicateNearTextRequiresSameSourceWindowAndStrictSimilarity(t *testing.T) {
	t.Parallel()

	baseTokens := numberedTokens("common", 100)
	candidateTokens := append(append([]string{}, baseTokens...), "candidateextraone", "candidateextratwo")
	content := mustNormalize(t, 20, "https://example.test/new", "Same, Headline!", strings.Join(baseTokens, " "))
	candidate := candidateFor(t, 70, 20, "https://example.test/old", strings.Repeat("a", 64), "same headline", strings.Join(candidateTokens, " "), content.PublishedAt.Add(-24*time.Hour))

	decision, err := DecideDuplicate(content, []ingestiondomain.ContentCandidate{candidate})
	if err != nil {
		t.Fatalf("DecideDuplicate(same source) error = %v", err)
	}
	if decision.Status != ingestiondomain.ContentStatusDuplicate || decision.DuplicateOfID == nil || *decision.DuplicateOfID != 70 || decision.Reason != ingestiondomain.DedupeReasonNearText || decision.Version != ingestiondomain.DedupeVersionNearText {
		t.Fatalf("near text decision = %#v, want inclusive 0.98 same-source duplicate", decision)
	}

	for _, test := range []struct {
		name      string
		candidate ingestiondomain.ContentCandidate
	}{
		{name: "cross_source_independent_report", candidate: replaceCandidate(candidate, func(value *ingestiondomain.ContentCandidate) { value.SourceConnectionID = 21 })},
		{name: "outside_24_hour_window", candidate: replaceCandidate(candidate, func(value *ingestiondomain.ContentCandidate) {
			value.PublishedAt = content.PublishedAt.Add(-24*time.Hour - time.Nanosecond)
		})},
		{name: "different_title_tokens", candidate: replaceCandidate(candidate, func(value *ingestiondomain.ContentCandidate) { value.TitleTokens = []string{"different", "headline"} })},
		{name: "empty_body_tokens", candidate: replaceCandidate(candidate, func(value *ingestiondomain.ContentCandidate) { value.BodyTokens = nil })},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, err := DecideDuplicate(content, []ingestiondomain.ContentCandidate{test.candidate})
			if err != nil {
				t.Fatalf("DecideDuplicate() error = %v", err)
			}
			if decision.Status != ingestiondomain.ContentStatusActive || decision.DuplicateOfID != nil || decision.Reason != "" || decision.Version != "" {
				t.Fatalf("decision = %#v, want independent active Content", decision)
			}
		})
	}
}

func TestDecideDuplicateKeepsCrossLanguageReportsAndSameSourceRetryIsExact(t *testing.T) {
	t.Parallel()

	english := mustNormalize(t, 41, "https://example.test/english", "Climate research", "independent English reporting")
	crossLanguage := candidateFor(t, 9, 41, "https://example.test/chinese", strings.Repeat("b", 64), "气候 研究", "独立 中文 报道", english.PublishedAt)
	decision, err := DecideDuplicate(english, []ingestiondomain.ContentCandidate{crossLanguage})
	if err != nil {
		t.Fatalf("DecideDuplicate(cross language) error = %v", err)
	}
	if decision.Status != ingestiondomain.ContentStatusActive {
		t.Fatalf("cross-language decision = %#v, want active", decision)
	}

	retry := candidateFor(t, 10, 41, english.CanonicalURL, strings.Repeat("c", 64), "old title", "old body", english.PublishedAt)
	decision, err = DecideDuplicate(english, []ingestiondomain.ContentCandidate{retry})
	if err != nil {
		t.Fatalf("DecideDuplicate(retry) error = %v", err)
	}
	if decision.Status != ingestiondomain.ContentStatusDuplicate || decision.DuplicateOfID == nil || *decision.DuplicateOfID != 10 || decision.Reason != ingestiondomain.DedupeReasonExactURL {
		t.Fatalf("same-source retry decision = %#v, want exact URL duplicate", decision)
	}
}

func TestDecideDuplicateReturnsActiveWhenNoCandidateMatches(t *testing.T) {
	t.Parallel()

	content := mustNormalize(t, 1, "https://example.test/current", "Current", "current body")
	candidate := candidateFor(t, 2, 1, "https://example.test/older", strings.Repeat("d", 64), "different", "unrelated body", content.PublishedAt)
	decision, err := DecideDuplicate(content, []ingestiondomain.ContentCandidate{candidate})
	if err != nil {
		t.Fatalf("DecideDuplicate() error = %v", err)
	}
	if decision != (ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive}) {
		t.Fatalf("decision = %#v, want active", decision)
	}
}

func mustNormalize(t *testing.T, sourceID int64, rawURL, title, body string) ingestiondomain.NormalizedContent {
	t.Helper()
	item := capturedItemForNormalization(rawURL, title, "author")
	item.Body = body
	content, err := NormalizeCapturedItem(item, sourceID)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem() error = %v", err)
	}
	return content
}

func candidateFor(t *testing.T, id, sourceID int64, canonicalURL, dedupeKey, title, body string, publishedAt time.Time) ingestiondomain.ContentCandidate {
	t.Helper()
	return ingestiondomain.ContentCandidate{
		ID:                 id,
		SourceConnectionID: sourceID,
		PublishedAt:        publishedAt,
		TitleTokens:        tokensForTest(title),
		BodyTokens:         tokensForTest(body),
		CanonicalURL:       canonicalURL,
		DedupeKey:          dedupeKey,
	}
}

func tokensForTest(value string) []string {
	return strings.Fields(strings.ToLower(value))
}

func numberedTokens(prefix string, count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = fmt.Sprintf("%s%03d", prefix, index)
	}
	return values
}

func replaceCandidate(candidate ingestiondomain.ContentCandidate, change func(*ingestiondomain.ContentCandidate)) ingestiondomain.ContentCandidate {
	change(&candidate)
	return candidate
}
