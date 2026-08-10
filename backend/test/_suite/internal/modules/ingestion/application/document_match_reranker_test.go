package application

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestRankSignalDocumentMatchRerankerProducesBoundedDeterministicProbabilities(t *testing.T) {
	t.Parallel()

	reranker := NewRankSignalDocumentMatchReranker()
	query := RerankDocumentMatchesQuery{
		MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19,
		MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion:          CanonicalDocumentMatchRerankerVersion,
		CalibrationVersion:       CanonicalDocumentMatchCalibrationVersion,
		CalibrationSlope:         1, CalibrationIntercept: 0,
		Candidates: []HybridRecallCandidateDTO{
			{DocumentVersionID: 17, Signals: []RecallSignalDTO{
				{Channel: "lexical", Rank: 1, RawScore: .9, AlgorithmVersion: LexicalRecallAlgorithmVersion},
				{Channel: "semantic", Rank: 1, RawScore: .8, AlgorithmVersion: SemanticRecallAlgorithmVersion},
				{Channel: "structured", Rank: 1, RawScore: 3, AlgorithmVersion: StructuredRecallAlgorithmVersion},
			}},
			{DocumentVersionID: 18, Signals: []RecallSignalDTO{
				{Channel: "lexical", Rank: 100, RawScore: .1, AlgorithmVersion: LexicalRecallAlgorithmVersion},
			}},
		},
	}
	original := cloneHybridRecallCandidates(query.Candidates)
	first, err := reranker.RerankDocumentMatches(context.Background(), query)
	if err != nil {
		t.Fatalf("RerankDocumentMatches() error = %v", err)
	}
	second, err := reranker.RerankDocumentMatches(context.Background(), query)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic rerank = %#v / %#v / %v", first, second, err)
	}
	if !reflect.DeepEqual(query.Candidates, original) {
		t.Fatal("reranker mutated caller candidates")
	}
	if len(first) != 2 || first[0].DocumentVersionID != 17 || first[1].DocumentVersionID != 18 ||
		first[0].RelevanceProbability < .90 || first[0].RelevanceProbability > 1 ||
		first[1].RelevanceProbability < 0 || first[1].RelevanceProbability >= .10 || first[0].Degraded || first[1].Degraded {
		t.Fatalf("reranked probabilities = %#v", first)
	}
	if !containsDocumentMatchReason(first[0].ReasonCodes, "rank_signal_probability") ||
		!containsDocumentMatchReason(first[0].ReasonCodes, "three_channel_agreement") ||
		!containsDocumentMatchReason(first[1].ReasonCodes, "single_channel_only") {
		t.Fatalf("reason codes = %#v / %#v", first[0].ReasonCodes, first[1].ReasonCodes)
	}
}

func TestRankSignalDocumentMatchRerankerRejectsProfileOrSignalDrift(t *testing.T) {
	t.Parallel()

	valid := RerankDocumentMatchesQuery{
		MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19,
		MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion:          CanonicalDocumentMatchRerankerVersion,
		CalibrationVersion:       CanonicalDocumentMatchCalibrationVersion,
		CalibrationSlope:         1, CalibrationIntercept: 0,
		Candidates: []HybridRecallCandidateDTO{{DocumentVersionID: 17, Signals: []RecallSignalDTO{
			{Channel: "lexical", Rank: 1, RawScore: .5, AlgorithmVersion: LexicalRecallAlgorithmVersion},
		}}},
	}
	for name, mutate := range map[string]func(*RerankDocumentMatchesQuery){
		"reranker version":    func(value *RerankDocumentMatchesQuery) { value.RerankerVersion = "foreign-v1" },
		"calibration version": func(value *RerankDocumentMatchesQuery) { value.CalibrationVersion = "foreign-v1" },
		"calibration slope":   func(value *RerankDocumentMatchesQuery) { value.CalibrationSlope = 0 },
		"duplicate channel": func(value *RerankDocumentMatchesQuery) {
			value.Candidates[0].Signals = append(value.Candidates[0].Signals, value.Candidates[0].Signals[0])
		},
		"invalid score": func(value *RerankDocumentMatchesQuery) { value.Candidates[0].Signals[0].RawScore = math.NaN() },
	} {
		t.Run(name, func(t *testing.T) {
			query := valid
			query.Candidates = cloneHybridRecallCandidates(valid.Candidates)
			mutate(&query)
			if _, err := NewRankSignalDocumentMatchReranker().RerankDocumentMatches(context.Background(), query); !errors.Is(err, ErrInvalidDocumentMatchContract) {
				t.Fatalf("RerankDocumentMatches() error = %v", err)
			}
		})
	}
}
