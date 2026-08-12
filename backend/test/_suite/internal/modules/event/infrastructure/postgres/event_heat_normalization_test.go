package postgres

import (
	"math"
	"testing"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
)

func TestNormalizeEventMetricEngagementUsesWindowDeltaInsteadOfEventInternalMaximum(t *testing.T) {
	baseline, latest := int64(0), int64(10)
	key := eventdomain.MetricPopulationKey{SourceConnectionID: 4, ContentType: "post"}
	capability := eventdomain.MetricCapability{
		SourceConnectionID: 4, SourceType: "x", ProfileID: 2, ProfileRecordVer: 1,
		ProfileVersion: "x-public-v1", SupportsLikes: true,
		IndependenceStrategy: "author", CredibilityWeight: 1, MaxSingleItemContribution: 100,
	}
	engagement, fallback, err := normalizeEventMetricEngagement([]eventMetricEvidence{{
		Evidence: eventdomain.MetricEvidence{ContentID: 8, SourceConnectionID: 4, ContentType: "post",
			Baseline: eventdomain.MetricCounts{Likes: &baseline}, Latest: eventdomain.MetricCounts{Likes: &latest}},
		Capability: capability,
	}}, map[eventdomain.MetricPopulationKey]eventdomain.MetricPopulation{
		key: {MetricPopulationKey: key, Deltas: []eventdomain.MetricCounts{{Likes: &latest}}},
	})
	if err != nil || engagement == nil || !fallback {
		t.Fatalf("engagement/fallback/error = %v/%v/%v", engagement, fallback, err)
	}
	want := math.Log1p(10) / math.Log1p(1000)
	if math.Abs(*engagement-want) > 0.0000001 || *engagement >= 1 {
		t.Fatalf("normalized engagement = %.9f, want %.9f and not a constant 1", *engagement, want)
	}
}

func TestNormalizeEventMetricEngagementPreservesMissingBaseline(t *testing.T) {
	latest := int64(10)
	capability := eventdomain.MetricCapability{
		SourceConnectionID: 4, SourceType: "x", ProfileID: 2, ProfileRecordVer: 1,
		ProfileVersion: "x-public-v1", SupportsLikes: true,
		IndependenceStrategy: "author", CredibilityWeight: 1, MaxSingleItemContribution: 100,
	}
	engagement, fallback, err := normalizeEventMetricEngagement([]eventMetricEvidence{{
		Evidence: eventdomain.MetricEvidence{ContentID: 8, SourceConnectionID: 4, ContentType: "post",
			Latest: eventdomain.MetricCounts{Likes: &latest}}, Capability: capability,
	}}, map[eventdomain.MetricPopulationKey]eventdomain.MetricPopulation{})
	if err != nil || engagement != nil || fallback {
		t.Fatalf("missing baseline engagement/fallback/error = %v/%v/%v", engagement, fallback, err)
	}
}
