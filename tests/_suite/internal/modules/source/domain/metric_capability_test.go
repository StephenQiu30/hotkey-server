package domain

import "testing"

func TestMetricCapabilityProfileRequiresExplicitSupportedMetric(t *testing.T) {
	profile := validMetricCapabilityProfile()
	profile.SupportsViews, profile.SupportsLikes, profile.SupportsComments, profile.SupportsShares = false, false, false, false
	if err := profile.ValidateDraft(); err == nil {
		t.Fatal("ValidateDraft() accepted a profile with no supported metrics")
	}
}

func TestMetricCapabilityProfileRejectsInvalidScoringBounds(t *testing.T) {
	profile := validMetricCapabilityProfile()
	profile.CredibilityWeight = 1.1
	if err := profile.ValidateDraft(); err == nil {
		t.Fatal("ValidateDraft() accepted credibility above one")
	}
	profile = validMetricCapabilityProfile()
	profile.MaxSingleItemContribution = 0
	if err := profile.ValidateDraft(); err == nil {
		t.Fatal("ValidateDraft() accepted non-positive item contribution cap")
	}
}

func validMetricCapabilityProfile() MetricCapabilityProfile {
	return MetricCapabilityProfile{
		SourceType:                SourceTypeRSS,
		ProfileVersion:            "v1",
		SupportsViews:             true,
		IndependenceStrategy:      IndependenceBySourceConnection,
		NormalizationWindowHours:  24,
		CredibilityWeight:         0.8,
		MaxSingleItemContribution: 50,
	}
}
