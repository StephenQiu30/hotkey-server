package application_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
)

type eventHeatGolden struct {
	Cases []eventHeatGoldenCase `json:"cases"`
}

type eventHeatGoldenCase struct {
	Name                      string                               `json:"name"`
	Coverage                  string                               `json:"coverage"`
	ProfileVersion            string                               `json:"profile_version"`
	Weights                   eventapplication.EventHeatWeightsDTO `json:"weights"`
	TemporalBaselineAvailable bool                                 `json:"temporal_baseline_available"`
	NormalizationFallback     bool                                 `json:"normalization_fallback"`
	NormalizedEngagement      *float64                             `json:"normalized_engagement"`
	Expected                  eventHeatGoldenExpected              `json:"expected"`
}

type eventHeatGoldenExpected struct {
	HeatScore            float64  `json:"heat_score"`
	Velocity             float64  `json:"velocity"`
	Acceleration         float64  `json:"acceleration"`
	Coverage             float64  `json:"coverage"`
	NormalizedEngagement *float64 `json:"normalized_engagement"`
	Recency              float64  `json:"recency"`
	AvailableWeight      float64  `json:"available_weight"`
	WarmingUp            bool     `json:"warming_up"`
	ReasonCodes          []string `json:"reason_codes"`
}

func TestEventHeatV2GoldenCoversProfilesUnknownMetricsAndWarmingUp(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "event-heat", "v2", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture eventHeatGolden
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	wantedCoverage := map[string]bool{
		"complete": false, "missing_engagement": false, "warming_up": false,
		"normalization_fallback": false, "profile_version": false,
	}
	endedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	for index, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			if _, exists := wantedCoverage[testCase.Coverage]; !exists || wantedCoverage[testCase.Coverage] {
				t.Fatalf("missing or duplicate coverage label %q", testCase.Coverage)
			}
			wantedCoverage[testCase.Coverage] = true
			repository := &eventHeatRepositoryFake{target: eventapplication.EventHeatTargetDTO{
				MicroEventID: 7, MicroEventVersion: 3, HeatProfileID: int64(index + 1),
				HeatProfileVersion: testCase.ProfileVersion, Weights: testCase.Weights,
				WindowStartedAt: endedAt.Add(-24 * time.Hour), WindowEndedAt: endedAt,
				IndependentLineageRoots: 4, ReportsInWindow: 8, ReportsInPreviousWindow: 4,
				ReportsInPriorWindow: 2, PublisherCoverage: 3, SourceTypeCoverage: 2,
				NormalizedEngagement:      testCase.NormalizedEngagement,
				NormalizationFallback:     testCase.NormalizationFallback,
				TemporalBaselineAvailable: testCase.TemporalBaselineAvailable, AgeHours: 2,
			}}
			service, err := eventapplication.NewEventHeatService(repository)
			if err != nil {
				t.Fatal(err)
			}
			first, err := service.Calculate(context.Background(), eventapplication.CalculateEventHeatCommand{
				MicroEventID: 7, WindowHours: 24, WindowEndedAt: endedAt,
			})
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.Calculate(context.Background(), eventapplication.CalculateEventHeatCommand{
				MicroEventID: 7, WindowHours: 24, WindowEndedAt: endedAt,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := first.Snapshot
			if got.HeatProfileVersion != testCase.ProfileVersion || got.HeatScore != testCase.Expected.HeatScore ||
				got.Velocity != testCase.Expected.Velocity || got.Acceleration != testCase.Expected.Acceleration ||
				got.Coverage != testCase.Expected.Coverage || !equalGoldenFloat(got.NormalizedEngagement, testCase.Expected.NormalizedEngagement) ||
				got.Recency != testCase.Expected.Recency || got.AvailableWeight != testCase.Expected.AvailableWeight ||
				got.WarmingUp != testCase.Expected.WarmingUp || !slices.Equal(got.ReasonCodes, testCase.Expected.ReasonCodes) ||
				second.Snapshot.HeatScore != got.HeatScore || second.Snapshot.AvailableWeight != got.AvailableWeight {
				t.Fatalf("snapshot=%#v replay=%#v want=%#v", got, second.Snapshot, testCase.Expected)
			}
		})
	}
	for coverage, found := range wantedCoverage {
		if !found {
			t.Errorf("fixture is missing %s coverage", coverage)
		}
	}
}

func equalGoldenFloat(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
