package application

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
)

func TestPlan009BoundedMultilingualRelevance(t *testing.T) {
	t.Parallel()
	reader := relevanceCandidateFake{candidates: plan009Candidates(21)}
	embeddings := relevanceEmbeddingFake{neighbors: plan009Neighbors(10, 21)}
	service, err := NewCandidateRecallService(&reader, &embeddings)
	if err != nil {
		t.Fatalf("NewCandidateRecallService() error = %v", err)
	}
	request := plan009ScoreRequest(strings.Repeat("a", 64))
	first, err := service.Score(context.Background(), request)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	if len(first) != 20 {
		t.Fatalf("Score() candidates = %d, want bounded union of 20", len(first))
	}
	if reader.sourceLimit != 8 || reader.lexicalLimit != 12 || embeddings.limit != 12 || len(reader.loaded) != 20 {
		t.Fatalf("candidate path limits/load = source:%d lexical:%d vector:%d batch:%d, want 8/12/12/20", reader.sourceLimit, reader.lexicalLimit, embeddings.limit, len(reader.loaded))
	}
	if reader.sourceCalls != 1 || reader.lexicalCalls != 1 || reader.loadCalls != 1 || embeddings.activeCalls != 1 || embeddings.nearestCalls != 1 {
		t.Fatalf("bounded query calls = source:%d lexical:%d batch:%d vector-active:%d vector-nearest:%d, want 1/1/1/1/1", reader.sourceCalls, reader.lexicalCalls, reader.loadCalls, embeddings.activeCalls, embeddings.nearestCalls)
	}
	if got, want := scoredIDs(first), []int64{5, 6, 7, 8, 1, 2, 3, 4, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deterministic candidate order = %v, want %v", got, want)
	}
	if first[0].ScoringVersion != relevanceDegradedVersion || !first[0].Degraded || first[0].Factors.Semantic != nil {
		t.Fatalf("candidate without vector = %#v, want deterministic degraded score", first[0])
	}
	if first[9].ScoringVersion != relevanceVersion || first[9].Degraded || first[9].Factors.Semantic == nil {
		t.Fatalf("candidate with exact-space vector = %#v, want full relevance-v1", first[9])
	}
	if candidate := scoreByMonitorID(t, first, 2); !candidate.HardVeto || candidate.Decision != ingestiondomain.MatchDecisionRejected || !contains(candidate.ExcludedTerms, "forbidden") {
		t.Fatalf("exclude hard veto = %#v, want rejected excluded candidate", candidate)
	}
	if candidate := scoreByMonitorID(t, first, 3); !candidate.HardVeto || !contains(candidate.ReasonCodes, "entity_conflict") {
		t.Fatalf("homonym entity conflict = %#v, want hard veto", candidate)
	}
	if candidate := scoreByMonitorID(t, first, 4); len(candidate.MatchedTerms) != 1 || candidate.MatchedTerms[0] != "gpt" {
		t.Fatalf("approved AI duplicate term = %#v, want one gpt match", candidate)
	}
	if candidate := scoreByMonitorID(t, first, 5); !contains(candidate.MatchedTerms, "中文发布") {
		t.Fatalf("Chinese phrase result = %#v, want NFC Chinese phrase match", candidate)
	}
	assertPlan009FixtureQuality(t, first)

	second, err := service.Score(context.Background(), request)
	if err != nil {
		t.Fatalf("Score(retry) error = %v", err)
	}
	if got, want := scoredIDs(second), scoredIDs(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("retry candidate order = %v, want %v", got, want)
	}
	changed := request
	changed.Content.DedupeKey = strings.Repeat("b", 64)
	afterDedupeChange, err := service.Score(context.Background(), changed)
	if err != nil {
		t.Fatalf("Score(dedupe change) error = %v", err)
	}
	if afterDedupeChange[0].InputHash == first[0].InputHash {
		t.Fatal("dedupe_key change reused relevance input hash")
	}
	for name, change := range map[string]func(*RelevanceScoreRequest){
		"region":             func(value *RelevanceScoreRequest) { value.Content.Region = "CN" },
		"canonical_host":     func(value *RelevanceScoreRequest) { value.Content.CanonicalURL = "https://other.example.test/item" },
		"author_external_id": func(value *RelevanceScoreRequest) { value.Content.AuthorExternalID = "other-author" },
		"author_name":        func(value *RelevanceScoreRequest) { value.Content.AuthorName = "Other Author" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			change(&changed)
			scored, err := service.Score(context.Background(), changed)
			if err != nil {
				t.Fatalf("Score(%s change) error = %v", name, err)
			}
			if scored[0].InputHash == first[0].InputHash {
				t.Fatalf("%s change reused relevance input hash", name)
			}
		})
	}
}

type relevanceCandidateFake struct {
	candidates                           []ingestiondomain.RelevanceCandidate
	sourceLimit, lexicalLimit            int
	sourceCalls, lexicalCalls, loadCalls int
	loaded                               []int64
}

func (fake *relevanceCandidateFake) SourceCandidates(_ context.Context, _ int64, limit int) ([]ingestiondomain.RelevanceCandidateHit, error) {
	fake.sourceCalls++
	fake.sourceLimit = limit
	return hits(1, 8, 0), nil
}

func (fake *relevanceCandidateFake) LexicalCandidates(_ context.Context, _ []string, limit int) ([]ingestiondomain.RelevanceCandidateHit, error) {
	fake.lexicalCalls++
	fake.lexicalLimit = limit
	return hits(5, 16, 100), nil
}

func (fake *relevanceCandidateFake) LoadRelevanceCandidates(_ context.Context, ids []int64) ([]ingestiondomain.RelevanceCandidate, error) {
	fake.loadCalls++
	fake.loaded = append([]int64(nil), ids...)
	allowed := map[int64]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	result := make([]ingestiondomain.RelevanceCandidate, 0, len(ids))
	for _, candidate := range fake.candidates {
		if _, ok := allowed[candidate.MonitorID]; ok {
			result = append(result, candidate)
		}
	}
	return result, nil
}

type relevanceEmbeddingFake struct {
	neighbors                        []intelligenceapplication.EmbeddingNeighbor
	limit, activeCalls, nearestCalls int
}

func (fake *relevanceEmbeddingFake) ActiveContent(_ context.Context, _ int64, space intelligenceapplication.EmbeddingSpace) (intelligenceapplication.ActiveEmbedding, bool, error) {
	fake.activeCalls++
	return intelligenceapplication.ActiveEmbedding{EmbeddingSpace: space, Vector: []float32{1}}, true, nil
}

func (fake *relevanceEmbeddingFake) NearestMonitors(_ context.Context, _ intelligenceapplication.ActiveEmbedding, limit int) ([]intelligenceapplication.EmbeddingNeighbor, error) {
	fake.nearestCalls++
	fake.limit = limit
	return fake.neighbors, nil
}

func plan009ScoreRequest(dedupeKey string) RelevanceScoreRequest {
	profile := ModelProfileReference{ID: 9, Version: 3, ModelVersion: "embedding-v1"}
	return RelevanceScoreRequest{Content: RelevanceContent{
		ID: 99, SourceConnectionID: 88, DedupeKey: dedupeKey, Language: "en", Region: "US",
		Title: "OpenAI launches GPT 中文发布", Excerpt: "Apple fruit rumors are forbidden.", CanonicalURL: "https://news.example.test/item",
		AuthorExternalID: "openai", AuthorName: "OpenAI News",
	}, EmbeddingProfile: &profile}
}

func plan009Candidates(count int) []ingestiondomain.RelevanceCandidate {
	result := make([]ingestiondomain.RelevanceCandidate, 0, count)
	for id := 1; id <= count; id++ {
		candidate := ingestiondomain.RelevanceCandidate{MonitorID: int64(id), MonitorConfigVersionID: int64(100 + id), ConfigHash: strings.Repeat("c", 64), RelevanceThreshold: 75,
			Rules: []ingestiondomain.RelevanceRule{{ID: int64(id * 10), RuleType: "keyword", Operator: "contains", Value: "OpenAI", Weight: 100, Origin: "user"}}}
		switch id {
		case 2:
			candidate.Rules = []ingestiondomain.RelevanceRule{{ID: 20, RuleType: "exclude_keyword", Operator: "contains", Value: "forbidden", Weight: 0, Origin: "user"}}
		case 3:
			candidate.Rules = []ingestiondomain.RelevanceRule{{ID: 30, RuleType: "keyword", Operator: "contains", Value: "Apple", Weight: 100, Origin: "user"}, {ID: 31, RuleType: "entity", Operator: "contains", Value: "Apple Inc", Weight: 100, Origin: "user"}}
		case 4:
			candidate.Rules = []ingestiondomain.RelevanceRule{{ID: 40, RuleType: "keyword", Operator: "contains", Value: "GPT", Weight: 100, Origin: "ai"}, {ID: 41, RuleType: "phrase", Operator: "contains", Value: "GPT", Weight: 100, Origin: "user"}}
		case 5:
			candidate.Rules = []ingestiondomain.RelevanceRule{{ID: 50, RuleType: "phrase", Operator: "contains", Value: "中文发布", Weight: 100, Origin: "user"}}
		}
		result = append(result, candidate)
	}
	return result
}

func plan009Neighbors(first, last int) []intelligenceapplication.EmbeddingNeighbor {
	result := make([]intelligenceapplication.EmbeddingNeighbor, 0, last-first+1)
	for id := first; id <= last; id++ {
		result = append(result, intelligenceapplication.EmbeddingNeighbor{TargetID: int64(id), Distance: math.Abs(float64(id-first)) / 100})
	}
	return result
}

func hits(first, last int, score float64) []ingestiondomain.RelevanceCandidateHit {
	result := make([]ingestiondomain.RelevanceCandidateHit, 0, last-first+1)
	for id := first; id <= last; id++ {
		result = append(result, ingestiondomain.RelevanceCandidateHit{MonitorID: int64(id), LexicalScore: score - float64(id)})
	}
	return result
}

func scoredIDs(values []ScoredRelevanceCandidate) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		result = append(result, value.MonitorID)
	}
	return result
}

func scoreByMonitorID(t *testing.T, values []ScoredRelevanceCandidate, id int64) ScoredRelevanceCandidate {
	t.Helper()
	for _, value := range values {
		if value.MonitorID == id {
			return value
		}
	}
	t.Fatalf("candidate %d missing from %#v", id, values)
	return ScoredRelevanceCandidate{}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func assertPlan009FixtureQuality(t *testing.T, values []ScoredRelevanceCandidate) {
	t.Helper()
	if len(values) != 20 {
		t.Fatalf("fixed relevance fixture size = %d, want 20", len(values))
	}
	relevant := 0
	falsePositiveExclusions := 0
	for _, value := range values {
		if value.Decision != ingestiondomain.MatchDecisionRejected {
			relevant++
		}
		if len(value.ExcludedTerms) > 0 && value.Decision != ingestiondomain.MatchDecisionRejected {
			falsePositiveExclusions++
		}
	}
	precisionAt20 := float64(relevant) / float64(len(values)) * 100
	falsePositiveExclusionRate := float64(falsePositiveExclusions) / float64(len(values)) * 100
	if precisionAt20 < 80 {
		t.Fatalf("fixed fixture P@20 = %.2f%%, want >= 80%%", precisionAt20)
	}
	if falsePositiveExclusionRate >= 2 {
		t.Fatalf("fixed fixture exclusion false-positive rate = %.2f%%, want < 2%%", falsePositiveExclusionRate)
	}
}
