package application

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type clusteringEvaluationFixture struct {
	Dataset          string                     `json:"dataset"`
	DatasetVersion   string                     `json:"dataset_version"`
	AlgorithmVersion string                     `json:"algorithm_version"`
	TimeRange        clusteringEvaluationRange  `json:"time_range"`
	Labeling         clusteringEvaluationLabels `json:"labeling"`
	Cases            []clusteringEvaluationCase `json:"cases"`
}

type clusteringEvaluationRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type clusteringEvaluationLabels struct {
	Protocol     string `json:"protocol"`
	ConflictRule string `json:"conflict_rule"`
}

type clusteringEvaluationCase struct {
	Name          string                           `json:"name"`
	ContentID     int64                            `json:"content_id"`
	Sample        clusteringEvaluationSample       `json:"sample"`
	Coverage      []string                         `json:"coverage"`
	Candidates    []clusteringEvaluationCandidate  `json:"candidates"`
	Scores        map[string]domain.ScoreBreakdown `json:"scores"`
	HardConflicts map[string]bool                  `json:"hard_conflicts"`
	Expected      string                           `json:"expected"`
}

type clusteringEvaluationSample struct {
	EventGroup      string `json:"event_group"`
	Split           string `json:"split"`
	Language        string `json:"language"`
	PublishedAt     string `json:"published_at"`
	SourceName      string `json:"source_name"`
	SourceURL       string `json:"source_url"`
	Title           string `json:"title"`
	AnnotationNote  string `json:"annotation_note"`
	ConflictOutcome string `json:"conflict_outcome"`
}

type clusteringEvaluationCandidate struct {
	EventID      int64   `json:"event_id"`
	EventKey     string  `json:"event_key"`
	Channel      string  `json:"channel"`
	Score        float64 `json:"score"`
	HardConflict bool    `json:"hard_conflict"`
}

func TestClusteringEvaluation(t *testing.T) {
	payload, err := os.ReadFile("testdata/clustering/v2/evaluation.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture clusteringEvaluationFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if fixture.Dataset != "event-clustering-acceptance" || fixture.DatasetVersion != "v2" || fixture.AlgorithmVersion != "v1" || len(fixture.Cases) < 12 {
		t.Fatalf("invalid fixture: %#v", fixture)
	}
	validateClusteringEvaluationCorpus(t, fixture)
	var truePositive, falsePositive, falseNegative int
	acceptanceLinks := 0
	acceptanceNonLinks := 0
	for _, testCase := range fixture.Cases {
		candidates := make([]domain.Candidate, 0, len(testCase.Candidates))
		for _, candidate := range testCase.Candidates {
			candidates = append(candidates, domain.Candidate{EventID: candidate.EventID, EventKey: candidate.EventKey, Channel: domain.CandidateChannel(candidate.Channel), Score: candidate.Score, HardConflict: candidate.HardConflict})
		}
		decisions, err := NewClusteringService().Evaluate(context.Background(), ClusteringInput{
			ContentID: testCase.ContentID, ClusteringVersion: fixture.AlgorithmVersion, FeatureInputHash: domain.FeatureInputHash("evaluation", testCase.Name),
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
		if testCase.Sample.Split != "acceptance" {
			continue
		}
		if testCase.Expected == "__review__" {
			acceptanceNonLinks++
			continue
		}
		expectedLink := testCase.Expected != "__new_event__"
		actualLink := actual != "__new_event__" && actual != "__review__"
		if expectedLink {
			acceptanceLinks++
		} else {
			acceptanceNonLinks++
		}
		switch {
		case expectedLink && actualLink && actual == testCase.Expected:
			truePositive++
		case actualLink:
			falsePositive++
		case expectedLink:
			falseNegative++
		}
	}
	if acceptanceLinks < 4 || acceptanceNonLinks < 3 {
		t.Fatalf("acceptance partition is too narrow: links=%d non_links=%d", acceptanceLinks, acceptanceNonLinks)
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

func validateClusteringEvaluationCorpus(t *testing.T, fixture clusteringEvaluationFixture) {
	t.Helper()
	if strings.TrimSpace(fixture.Labeling.Protocol) == "" || strings.TrimSpace(fixture.Labeling.ConflictRule) == "" {
		t.Fatal("corpus must record labeling and conflict-resolution rules")
	}
	start, err := time.Parse(time.RFC3339, fixture.TimeRange.Start)
	if err != nil {
		t.Fatalf("parse corpus start: %v", err)
	}
	end, err := time.Parse(time.RFC3339, fixture.TimeRange.End)
	if err != nil || !end.After(start) || end.Sub(start) < 180*24*time.Hour {
		t.Fatalf("corpus must span at least 180 days: start=%q end=%q err=%v", fixture.TimeRange.Start, fixture.TimeRange.End, err)
	}

	groupSplits := map[string]string{}
	languages := map[string]bool{}
	sourceNames := map[string]bool{}
	coverage := map[string]bool{}
	contentIDs := map[int64]bool{}
	for _, testCase := range fixture.Cases {
		if testCase.ContentID <= 0 || contentIDs[testCase.ContentID] || strings.TrimSpace(testCase.Name) == "" {
			t.Fatalf("invalid or duplicated corpus case: %#v", testCase)
		}
		contentIDs[testCase.ContentID] = true
		sample := testCase.Sample
		if strings.TrimSpace(sample.EventGroup) == "" || (sample.Split != "calibration" && sample.Split != "acceptance") || (sample.Language != "en" && sample.Language != "zh") || strings.TrimSpace(sample.SourceName) == "" || strings.TrimSpace(sample.Title) == "" || strings.TrimSpace(sample.AnnotationNote) == "" {
			t.Fatalf("case %q lacks provenance or annotation: %#v", testCase.Name, sample)
		}
		publishedAt, err := time.Parse(time.RFC3339, sample.PublishedAt)
		if err != nil || publishedAt.Before(start) || publishedAt.After(end) {
			t.Fatalf("case %q has published_at outside corpus range: %q (%v)", testCase.Name, sample.PublishedAt, err)
		}
		parsedURL, err := url.ParseRequestURI(sample.SourceURL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
			t.Fatalf("case %q has invalid source URL: %q", testCase.Name, sample.SourceURL)
		}
		if split, exists := groupSplits[sample.EventGroup]; exists && split != sample.Split {
			t.Fatalf("event group %q leaks across calibration and acceptance", sample.EventGroup)
		}
		groupSplits[sample.EventGroup] = sample.Split
		languages[sample.Language] = true
		sourceNames[sample.SourceName] = true
		for _, item := range testCase.Coverage {
			coverage[item] = true
		}
		if testCase.Expected == "__review__" && strings.TrimSpace(sample.ConflictOutcome) == "" {
			t.Fatalf("review case %q must preserve its conflict outcome", testCase.Name)
		}
	}
	if len(groupSplits) < 5 || !languages["en"] || !languages["zh"] || len(sourceNames) < 4 {
		t.Fatalf("corpus diversity is insufficient: groups=%d languages=%v sources=%d", len(groupSplits), languages, len(sourceNames))
	}
	requiredCoverage := []string{"cross_language", "same_entity_different_time", "same_entity_different_incident", "late_update", "ambiguous_human_review"}
	for _, item := range requiredCoverage {
		if !coverage[item] {
			t.Fatalf("corpus lacks required coverage %q", item)
		}
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
