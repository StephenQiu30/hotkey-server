package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type rankingFixture struct {
	Version                        string               `json:"version"`
	MinimumRelevance               float64              `json:"minimum_relevance"`
	MinimumRisingRecall            float64              `json:"minimum_rising_recall"`
	MaximumRisingFalsePositiveRate float64              `json:"maximum_rising_false_positive_rate"`
	ExpectedTop20                  []string             `json:"expected_top_20"`
	Items                          []rankingFixtureItem `json:"items"`
}

type rankingFixtureItem struct {
	ID                  string   `json:"id"`
	RelevanceScore      float64  `json:"relevance_score"`
	HeatScore           float64  `json:"heat_score"`
	TrendScore          float64  `json:"trend_score"`
	FreshnessHours      float64  `json:"freshness_hours"`
	HumanRising         bool     `json:"human_rising"`
	ExpectedRank        int      `json:"expected_rank"`
	ExpectedReasonCodes []string `json:"expected_reason_codes"`
}

func TestRankingEvaluationV1Top20Baseline(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "application", "testdata", "ranking", "v1", "top20.json"))
	if err != nil {
		t.Fatalf("read ranking fixture: %v", err)
	}
	var fixture rankingFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode ranking fixture: %v", err)
	}
	if fixture.Version != "ranking-v1" || len(fixture.ExpectedTop20) != 20 || len(fixture.Items) < 21 {
		t.Fatalf("invalid ranking fixture metadata: %#v", fixture)
	}

	type scoredItem struct {
		item    rankingFixtureItem
		score   float64
		reasons []string
	}
	scored := make([]scoredItem, 0, len(fixture.Items))
	for _, item := range fixture.Items {
		score, reasons, err := RankMonitorEvent(RankingInput{RelevanceScore: item.RelevanceScore, MinimumRelevance: fixture.MinimumRelevance, HeatScore: item.HeatScore, TrendScore: item.TrendScore, FreshnessHours: item.FreshnessHours})
		if err != nil {
			t.Fatalf("rank %s: %v", item.ID, err)
		}
		if !reflect.DeepEqual(reasons, item.ExpectedReasonCodes) {
			t.Errorf("%s reason codes = %#v, want %#v", item.ID, reasons, item.ExpectedReasonCodes)
		}
		scored = append(scored, scoredItem{item: item, score: score, reasons: reasons})
	}
	sort.Slice(scored, func(left, right int) bool {
		if scored[left].score == scored[right].score {
			return scored[left].item.ID < scored[right].item.ID
		}
		return scored[left].score > scored[right].score
	})

	actualTop20 := make([]string, 0, 20)
	var expectedRising, truePositive, predictedRising, falsePositive int
	for index, result := range scored {
		if got, want := index+1, result.item.ExpectedRank; got != want {
			t.Errorf("%s rank = %d, want %d", result.item.ID, got, want)
		}
		if index < 20 {
			actualTop20 = append(actualTop20, result.item.ID)
			predicted := containsReason(result.reasons, "rising")
			if result.item.HumanRising {
				expectedRising++
			}
			if predicted {
				predictedRising++
				if result.item.HumanRising {
					truePositive++
				} else {
					falsePositive++
				}
			}
		}
	}
	if !reflect.DeepEqual(actualTop20, fixture.ExpectedTop20) {
		t.Errorf("Top-20 = %#v, want %#v", actualTop20, fixture.ExpectedTop20)
	}
	if recall := float64(truePositive) / float64(expectedRising); recall < fixture.MinimumRisingRecall {
		t.Errorf("rising recall = %.2f, want >= %.2f", recall, fixture.MinimumRisingRecall)
	}
	if rate := float64(falsePositive) / float64(predictedRising); rate > fixture.MaximumRisingFalsePositiveRate {
		t.Errorf("rising false-positive rate = %.2f, want <= %.2f", rate, fixture.MaximumRisingFalsePositiveRate)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
