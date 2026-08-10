package domain

import "testing"

func TestHeatV2CountsIndependentLineageAndRenormalizesMissingEngagement(t *testing.T) {
	t.Parallel()
	engagement := .8
	complete, err := CalculateEventHeat(EventHeatInput{IndependentLineageRoots: 4, ReportsInWindow: 8,
		ReportsInPreviousWindow: 4, ReportsInPriorWindow: 2, PublisherCoverage: 3, SourceTypeCoverage: 2,
		NormalizedEngagement: &engagement, AgeHours: 2, ProfileVersion: "heat-v2"})
	if err != nil {
		t.Fatal(err)
	}
	missing, err := CalculateEventHeat(EventHeatInput{IndependentLineageRoots: 4, ReportsInWindow: 8,
		ReportsInPreviousWindow: 4, ReportsInPriorWindow: 2, PublisherCoverage: 3, SourceTypeCoverage: 2,
		AgeHours: 2, ProfileVersion: "heat-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if complete.Score <= 0 || missing.Score <= 0 || complete.IndependentLineageRoots != 4 || missing.AvailableWeight != .85 {
		t.Fatalf("heat complete/missing = %#v / %#v", complete, missing)
	}
	if len(missing.ReasonCodes) != 1 || missing.ReasonCodes[0] != "metrics_unavailable" {
		t.Fatalf("missing metric reasons = %#v", missing.ReasonCodes)
	}
}

func TestHeatV2RejectsInvalidMetricsWithoutCredibilityFallback(t *testing.T) {
	t.Parallel()
	invalid := -0.1
	if _, err := CalculateEventHeat(EventHeatInput{IndependentLineageRoots: 3, ReportsInWindow: 3,
		NormalizedEngagement: &invalid, ProfileVersion: "heat-v2"}); err == nil {
		t.Fatal("invalid engagement was accepted")
	}
	result, err := CalculateEventHeat(EventHeatInput{IndependentLineageRoots: 3, ReportsInWindow: 3,
		ProfileVersion: "heat-v2"})
	if err != nil || result.Score <= 0 {
		t.Fatalf("missing engagement should renormalize, got %#v / %v", result, err)
	}
}
