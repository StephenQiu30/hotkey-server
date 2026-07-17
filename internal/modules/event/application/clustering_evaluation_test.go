package application

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type clusteringEvaluationFixture struct {
	Version string                     `json:"version"`
	Cases   []clusteringEvaluationCase `json:"cases"`
}

type clusteringEvaluationCase struct {
	Name          string                           `json:"name"`
	ContentID     int64                            `json:"content_id"`
	Candidates    []clusteringEvaluationCandidate  `json:"candidates"`
	Scores        map[string]domain.ScoreBreakdown `json:"scores"`
	HardConflicts map[string]bool                  `json:"hard_conflicts"`
	Expected      string                           `json:"expected"`
}

type clusteringEvaluationCandidate struct {
	EventID      int64   `json:"event_id"`
	EventKey     string  `json:"event_key"`
	Channel      string  `json:"channel"`
	Score        float64 `json:"score"`
	HardConflict bool    `json:"hard_conflict"`
}

func TestClusteringEvaluation(t *testing.T) {
	payload, err := os.ReadFile("testdata/clustering/v1/evaluation.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture clusteringEvaluationFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if fixture.Version != "v1" || len(fixture.Cases) == 0 {
		t.Fatalf("invalid fixture: %#v", fixture)
	}
	var truePositive, falsePositive, falseNegative int
	for _, testCase := range fixture.Cases {
		candidates := make([]domain.Candidate, 0, len(testCase.Candidates))
		for _, candidate := range testCase.Candidates {
			candidates = append(candidates, domain.Candidate{EventID: candidate.EventID, EventKey: candidate.EventKey, Channel: domain.CandidateChannel(candidate.Channel), Score: candidate.Score, HardConflict: candidate.HardConflict})
		}
		decisions, err := NewClusteringService().Evaluate(context.Background(), ClusteringInput{
			ContentID: testCase.ContentID, ClusteringVersion: fixture.Version, FeatureInputHash: domain.FeatureInputHash("evaluation", testCase.Name),
			Candidates: candidates, Scores: testCase.Scores, HardConflicts: testCase.HardConflicts,
		})
		if err != nil {
			t.Fatalf("%s: Evaluate() error = %v", testCase.Name, err)
		}
		if len(candidates) > domain.MaxCandidates {
			t.Fatalf("%s: fixture exceeds candidate limit", testCase.Name)
		}
		actual := clusteringOutcome(decisions)
		if actual != testCase.Expected {
			t.Errorf("%s: outcome = %q, want %q", testCase.Name, actual, testCase.Expected)
		}
		if testCase.Expected == "__review__" {
			continue
		}
		expectedLink := testCase.Expected != "__new_event__"
		actualLink := actual != "__new_event__" && actual != "__review__"
		switch {
		case expectedLink && actualLink && actual == testCase.Expected:
			truePositive++
		case actualLink:
			falsePositive++
		case expectedLink:
			falseNegative++
		}
	}
	denominator := 2*truePositive + falsePositive + falseNegative
	if denominator == 0 {
		t.Fatal("evaluation fixture has no link decisions")
	}
	f1 := float64(2*truePositive) / float64(denominator)
	if f1 < .75 {
		t.Fatalf("clustering F1 = %.2f, want >= 0.75 (tp=%d fp=%d fn=%d)", f1, truePositive, falsePositive, falseNegative)
	}
}

func clusteringOutcome(decisions []domain.Decision) string {
	for _, decision := range decisions {
		if decision.Decision == domain.DecisionAccept {
			return decision.CandidateEventKey
		}
	}
	for _, decision := range decisions {
		if decision.Decision == domain.DecisionReview {
			return "__review__"
		}
	}
	return "__new_event__"
}
